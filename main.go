package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"ai-router/internal/config"
	"ai-router/internal/converters"
	"ai-router/internal/health"
	"ai-router/internal/login"
	"ai-router/internal/middleware"
	"ai-router/internal/models"
	"ai-router/internal/providers"
	"ai-router/internal/router"
	"ai-router/internal/setup"
)

// maxBodyBytes caps the size of an inbound request body. Prevents a single
// oversized request from exhausting server memory (DoS).
const maxBodyBytes int64 = 10 << 20 // 10 MiB

// defaultHTTPClient replaces http.DefaultClient with explicit transport
// timeouts so that connections to upstream AI providers cannot hang forever
// (notably on Windows where the default dialer can stall indefinitely on
// certain network configurations or when the system proxy is misconfigured).
var defaultHTTPClient = &http.Client{
	Timeout: 5 * time.Minute,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout:  60 * time.Second,
		IdleConnTimeout:        90 * time.Second,
		MaxIdleConns:           100,
		MaxIdleConnsPerHost:    10,
	},
}

// debugMode is set to true when gateway.debug: true in config.yaml.
// debugLog only prints when this flag is active.
var debugMode bool

func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf("[debug] "+format, args...)
	}
}

type server struct {
	cfg           *config.Config
	engine        *router.Engine
	reqConverter  *converters.AnthropicToOpenAIConverter
	respConverter *converters.OpenAIToAnthropicConverter
	limiter       *middleware.Limiter
	passthrough   *config.ProviderConfig
	monitor       *health.Monitor
	modelAggs     map[string]*aggregationState
}

type aggregationState struct {
	Config  config.ModelAggregation
	RRIndex int
	// mu guards RRIndex, which is read-then-incremented by SelectCandidate
	// on every concurrent request routed to this aggregation.
	mu sync.Mutex
}

// selectCandidateLocked serializes provider selection so the round-robin
// counter is not raced across concurrent requests sharing aggAS.
func (a *aggregationState) selectCandidateLocked(candidates []router.Candidate, strat router.Strategy) (*router.Candidate, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return router.SelectCandidate(candidates, strat, &a.RRIndex)
}

func main() {
	// Handle subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "setup":
			setup.Run()
			return
		case "server":
			// Fall through to server start below
		case "login":
			login.Run(os.Args[2:])
			return
		case "version", "--version", "-v":
			fmt.Println("aimux v0.1.3")
			return
		case "help", "--help", "-h":
			printUsage()
			return
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\\n\\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	} else {
		printUsage()
		return
	}

	startServer()
}

func startServer() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v — searched: cwd=%s, exe=%s", err,
			func() string { cwd, _ := os.Getwd(); return cwd }(),
			func() string { exe, _ := os.Executable(); return exe }())
	}
	if config.LoadedFrom != "" {
		log.Printf("[config] loaded from %s", config.LoadedFrom)
	}
	if cfg.Gateway.Debug {
		debugMode = true
		log.Printf("[config] debug mode enabled")
	}

	reqConverter := converters.NewAnthropicToOpenAIConverter(cfg.ModelMapping)
	respConverter := converters.NewOpenAIToAnthropicConverter()

	// Build router providers (exclude passthrough providers from conversion routing).
	var routerProviders []*router.Provider
	var passthrough *config.ProviderConfig
	for i := range cfg.Providers {
		pc := &cfg.Providers[i]
		if !pc.Enabled {
			continue
		}
		if pc.AutoModel {
			if m, err := resolveAutoModel(pc); err != nil {
				log.Printf("[config] provider %s auto model failed: %v", pc.Name, err)
			} else {
				log.Printf("[config] provider %s auto-selected model %s", pc.Name, m)
			}
		}
		if pc.Passthrough || (pc.Name == "anthropic" && getBoolEnv("ANTHROPIC_PASSTHROUGH", false)) {
			passthrough = pc
			continue
		}
		// Use CodexProvider for codex (ChatGPT OAuth) provider.
		var client router.ProviderClient
		if pc.Name == "codex" {
			codexClient, err := providers.NewCodexProvider(pc.Timeout)
			if err != nil {
				log.Printf("[config] codex provider skipped: %v", err)
				continue
			}
			client = codexClient
		} else {
			client = providers.NewOpenAIProvider(pc.BaseURL, pc.APIKey, pc.Timeout)
		}
		routerProviders = append(routerProviders, &router.Provider{
			Name:             pc.Name,
			BaseURL:          pc.BaseURL,
			Model:            pc.Model,
			Weight:           pc.Weight,
			Priority:         pc.Priority,
			MaxRPM:           pc.MaxRPM,
			FailureThreshold: pc.FailureThreshold,
			CooldownSeconds:  pc.CooldownSeconds,
			Passthrough:      pc.Passthrough,
			Client:           client,
		})
	}

	if len(routerProviders) == 0 {
		log.Fatal("no usable (non-passthrough) providers configured")
	}

	strategy := router.Strategy(cfg.Routing.Strategy)
	engine, err := router.NewEngine(routerProviders, strategy, cfg.Routing.MaxRetries)
	if err != nil {
		log.Fatalf("engine error: %v", err)
	}

	// Build model aggregation states.
	modelAggs := make(map[string]*aggregationState, len(cfg.ModelAggregations))
	for i := range cfg.ModelAggregations {
		agg := &cfg.ModelAggregations[i]
		modelAggs[agg.Name] = &aggregationState{Config: *agg}
		log.Printf("[config] model aggregation %q strategy=%s entries=%d",
			agg.Name, agg.Strategy, len(agg.Models))
	}

	srv := &server{
		cfg:           cfg,
		engine:        engine,
		reqConverter:  reqConverter,
		respConverter: respConverter,
		limiter:       middleware.NewLimiter(cfg.RateLimit.RPM, cfg.RateLimit.Burst),
		passthrough:   passthrough,
		modelAggs:     modelAggs,
	}

	monitor := health.NewMonitor(engine,
		time.Duration(cfg.CircuitBreaker.HealthCheckInterval)*time.Second,
		time.Duration(cfg.CircuitBreaker.CooldownSeconds)*time.Second,
		cfg.CircuitBreaker.FailureThreshold,
		func(name string, from, to router.CircuitState) {
			log.Printf("[health] provider %s state %s -> %s", name, from, to)
		},
	)
	srv.monitor = monitor
	monitor.Start()

	r := chi.NewRouter()

	// Public endpoints — no auth required (model discovery and health probes).
	r.Get("/v1/models", srv.handleModels)
	r.Get("/health", srv.handleHealth)

	// Protected endpoints — require a valid API key when keys are configured.
	r.Group(func(r chi.Router) {
		r.Use(srv.authMiddleware)
		r.Use(srv.rateLimitMiddleware)

		r.Post("/v1/messages", srv.handleMessages)
		r.Post("/v1/messages/count_tokens", srv.handleCountTokens)
		r.Post("/v1/chat/completions", srv.handleChatCompletions)
		r.Post("/v1/responses", srv.handleResponses)
		r.Get("/health/providers", srv.handleProviderHealth)
		r.Get("/admin/stats", srv.handleStats)
	})

	addr := cfg.Gateway.Host + ":" + strconv.Itoa(cfg.Gateway.Port)
	log.Printf("AI API Gateway listening on %s (strategy=%s, providers=%d)", addr, strategy, len(routerProviders))

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Listen for interrupt / termination signals and shut down gracefully so
	// in-flight (especially streaming) requests can drain instead of being
	// severed mid-response.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	errCh := make(chan error, 1)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		log.Fatalf("server error: %v", err)
	case sig := <-stop:
		log.Printf("received %s, shutting down...", sig)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	monitor.Stop()
	log.Printf("server stopped")
}

// ---- Middleware ----

func (s *server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.Auth.ValidAPIKeysEmpty() {
			key := r.Header.Get("x-api-key")
			if key == "" {
				if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
					key = strings.TrimPrefix(auth, "Bearer ")
				}
			}
			if !middleware.ValidateAPIKey(key, s.cfg.Auth.ValidAPIKeys) {
				writeError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.RateLimit.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		key := r.Header.Get("x-api-key")
		if key == "" {
			key = r.RemoteAddr
		}
		if !s.limiter.Allow(key) {
			writeError(w, http.StatusTooManyRequests, "rate_limit_error", "Rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- Handlers ----

func (s *server) handleMessages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "cannot read body")
		return
	}

	var req models.Request
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	debugLog("POST /v1/messages model=%s stream=%v msgs=%d", req.Model, req.Stream, len(req.Messages))

	// Aggregation model routing.
	if agg, ok := s.modelAggs[req.Model]; ok {
		debugLog("model %q routed to aggregation %q", req.Model, agg.Config.Name)
		openaiReq, err := s.reqConverter.Convert(&req)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		s.handleAggregated(w, r, openaiReq, agg)
		return
	}

	// Passthrough for native Claude models.
	if strings.HasPrefix(req.Model, "claude-") && s.passthrough != nil {
		debugLog("model %q routed to passthrough", req.Model)
		s.passthroughAnthropic(w, r, body)
		return
	}

	openaiReq, err := s.reqConverter.Convert(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.Stream {
		s.handleStream(w, r, openaiReq)
		return
	}

	s.handleNonStream(w, r, openaiReq)
}

func (s *server) handleNonStream(w http.ResponseWriter, r *http.Request, openaiReq *models.ChatCompletionRequest) {
	debugLog("direct non-stream request: model=%s", openaiReq.Model)
	resp, _, err := s.engine.Execute(r.Context(), *openaiReq)
	if err != nil {
		debugLog("direct non-stream error: %v", err)
		writeError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	anthropicResp, err := s.respConverter.ConvertNonStream(resp, openaiReq.Model)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anthropicResp)
}

func (s *server) handleStream(w http.ResponseWriter, r *http.Request, openaiReq *models.ChatCompletionRequest) {
	debugLog("direct stream request: model=%s", openaiReq.Model)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return
	}
	fw := &flushingWriter{w: w, f: flusher}
	sse := converters.NewSSEWriter(fw)

	attempted := map[string]bool{}
	ctx := r.Context()
	committed := false

	for attempt := 0; attempt <= s.cfg.Routing.MaxRetries; attempt++ {
		p, err := s.engine.SelectProvider(attempted)
		if err != nil {
			break
		}
		start := time.Now()
		ch, err := p.Client.ChatCompletionStream(ctx, *openaiReq)
		latency := float64(time.Since(start).Microseconds()) / 1000.0
		if err != nil {
			debugLog("stream attempt %d: provider=%s latency=%.1fms err=%v", attempt+1, p.Name, latency, err)
			attempted[p.Name] = true
			if pe, ok := err.(*router.ProviderError); ok {
				s.engine.RecordFailure(p, pe.Retryable)
			} else {
				s.engine.RecordFailure(p, true)
			}
			continue
		}
		debugLog("stream attempt %d: provider=%s latency=%.1fms stream started", attempt+1, p.Name, latency)
		// Commit the stream.
		streamErr := s.respConverter.WriteStream(ch, p.Model, sse)
		if streamErr == nil {
			s.engine.RecordSuccess(p, latency)
		} else {
			debugLog("stream write failed: %v", streamErr)
			s.engine.RecordFailure(p, true)
		}
		committed = true
		break
	}

	if !committed {
		sse.Write("error", map[string]interface{}{
			"type":  "error",
			"error": map[string]interface{}{"type": "api_error", "message": "all providers failed"},
		})
	}
}

func (s *server) passthroughAnthropic(w http.ResponseWriter, r *http.Request, body []byte) {
	pc := s.passthrough
	url := strings.TrimRight(pc.BaseURL, "/") + "/v1/messages"

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(pc.Timeout)*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", pc.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := defaultHTTPClient.Do(httpReq)
	if err != nil {
		debugLog("passthrough request error: %v", err)
		writeError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	defer resp.Body.Close()
	debugLog("passthrough response: status=%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}

	// Stream the raw SSE (Anthropic format) back to the client.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	io.Copy(w, resp.Body)
}

func (s *server) handleAggregated(w http.ResponseWriter, r *http.Request, openaiReq *models.ChatCompletionRequest, aggAS *aggregationState) {
	candidates := s.buildAggCandidates(aggAS.Config.Models)
	if len(candidates) == 0 {
		writeError(w, http.StatusBadGateway, "api_error", "no valid providers for this aggregation")
		return
	}

	if openaiReq.Stream {
		s.handleAggregatedStream(w, r, openaiReq, aggAS, candidates)
	} else {
		s.handleAggregatedNonStream(w, r, openaiReq, aggAS, candidates)
	}
}

func (s *server) handleAggregatedNonStream(w http.ResponseWriter, r *http.Request, openaiReq *models.ChatCompletionRequest, aggAS *aggregationState, candidates []router.Candidate) {
	ctx := r.Context()
	strat := router.Strategy(aggAS.Config.Strategy)
	remaining := candidates

	for attempt := 0; attempt < len(candidates); attempt++ {
		c, err := aggAS.selectCandidateLocked(remaining, strat)
		if err != nil {
			break
		}
		openaiReq.Model = c.Model
		debugLog("attempt %d: provider=%s model=%s", attempt+1, c.Provider.Name, c.Model)
		start := time.Now()
		resp, err := c.Provider.Client.ChatCompletion(ctx, *openaiReq)
		latency := float64(time.Since(start).Microseconds()) / 1000.0
		debugLog("attempt %d: latency=%.1fms err=%v", attempt+1, latency, err)

		if err == nil {
			s.engine.RecordSuccess(c.Provider, latency)
			anthropicResp, convErr := s.respConverter.ConvertNonStream(resp, c.Model)
			if convErr != nil {
				writeError(w, http.StatusInternalServerError, "api_error", convErr.Error())
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(anthropicResp)
			return
		}
		if pe, ok := err.(*router.ProviderError); ok {
			s.engine.RecordFailure(c.Provider, pe.Retryable)
		} else {
			s.engine.RecordFailure(c.Provider, true)
		}
		remaining = filterCandidates(remaining, c.Provider.Name)
	}
	writeError(w, http.StatusBadGateway, "api_error", "all aggregation entries failed")
}

func (s *server) handleAggregatedStream(w http.ResponseWriter, r *http.Request, openaiReq *models.ChatCompletionRequest, aggAS *aggregationState, candidates []router.Candidate) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return
	}
	fw := &flushingWriter{w: w, f: flusher}
	sse := converters.NewSSEWriter(fw)

	ctx := r.Context()
	strat := router.Strategy(aggAS.Config.Strategy)
	remaining := candidates
	committed := false

	for attempt := 0; attempt < len(candidates); attempt++ {
		c, err := aggAS.selectCandidateLocked(remaining, strat)
		if err != nil {
			break
		}
		openaiReq.Model = c.Model
		start := time.Now()
		ch, err := c.Provider.Client.ChatCompletionStream(ctx, *openaiReq)
		if err != nil {
			if pe, ok := err.(*router.ProviderError); ok {
				s.engine.RecordFailure(c.Provider, pe.Retryable)
			} else {
				s.engine.RecordFailure(c.Provider, true)
			}
			remaining = filterCandidates(remaining, c.Provider.Name)
			continue
		}
		streamErr := s.respConverter.WriteStream(ch, c.Model, sse)
		latency := float64(time.Since(start).Microseconds()) / 1000.0
		if streamErr == nil {
			s.engine.RecordSuccess(c.Provider, latency)
		} else {
			s.engine.RecordFailure(c.Provider, true)
		}
		committed = true
		break
	}

	if !committed {
		sse.Write("error", map[string]interface{}{
			"type":  "error",
			"error": map[string]interface{}{"type": "api_error", "message": "all aggregation entries failed"},
		})
	}
}

func (s *server) buildAggCandidates(entries []config.ModelAggEntry) []router.Candidate {
	var out []router.Candidate
	for _, e := range entries {
		p, ok := s.engine.Providers()[e.Provider]
		if !ok {
			log.Printf("[agg] provider %q not found, skipping", e.Provider)
			continue
		}
		out = append(out, router.Candidate{
			Provider: p,
			Model:    e.Model,
			Weight:   e.Weight,
		})
	}
	return out
}

func filterCandidates(candidates []router.Candidate, exclude string) []router.Candidate {
	out := make([]router.Candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Provider.Name != exclude {
			out = append(out, c)
		}
	}
	return out
}

func (s *server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	var req models.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON")
		return
	}
	total := estimateTokens(req.System)
	for _, m := range req.Messages {
		total += estimateTokens(m.Content)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"input_tokens": total})
}

func (s *server) handleModels(w http.ResponseWriter, r *http.Request) {
	data := []map[string]interface{}{}
	for _, agg := range s.cfg.ModelAggregations {
		data = append(data, map[string]interface{}{
			"id":       agg.Name,
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "aggregation",
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"object": "list", "data": data})
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *server) handleProviderHealth(w http.ResponseWriter, r *http.Request) {
	statuses := map[string]map[string]interface{}{}
	overall := "healthy"
	for name, p := range s.engine.Providers() {
		snap := p.Snapshot()
		fr := 0.0
		if snap.TotalRequests > 0 {
			fr = float64(snap.TotalFailures) / float64(snap.TotalRequests) * 100
		}
		status := "healthy"
		if snap.CircuitState == router.StateOpen {
			status = "down"
			overall = "degraded"
		} else if snap.CircuitState == router.StateHalfOpen {
			status = "degraded"
			overall = "degraded"
		}
		statuses[name] = map[string]interface{}{
			"status":         status,
			"circuit_state":  string(snap.CircuitState),
			"avg_latency_ms": round1(snap.AvgLatencyMs),
			"total_requests": snap.TotalRequests,
			"total_failures": snap.TotalFailures,
			"failure_rate":   round1(fr),
			"weight":         snap.Weight,
			"priority":       snap.Priority,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    overall,
		"strategy":  string(s.engine.Strategy()),
		"providers": statuses,
	})
}

func (s *server) handleStats(w http.ResponseWriter, r *http.Request) {
	totalReq := 0
	totalFail := 0
	for _, p := range s.engine.Providers() {
		snap := p.Snapshot()
		totalReq += snap.TotalRequests
		totalFail += snap.TotalFailures
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_requests": totalReq,
		"total_failures": totalFail,
		"providers":      len(s.engine.Providers()),
		"strategy":       string(s.engine.Strategy()),
	})
}

func (s *server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "cannot read body")
		return
	}

	var openaiReq models.ChatCompletionRequest
	if err := json.Unmarshal(body, &openaiReq); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}

	// Handle aggregation model.
	if agg, ok := s.modelAggs[openaiReq.Model]; ok {
		if openaiReq.Stream {
			s.handleAggregatedChatStream(w, r, &openaiReq, agg)
		} else {
			s.handleAggregatedChatNonStream(w, r, &openaiReq, agg)
		}
		return
	}

	if openaiReq.Stream {
		s.handleChatStream(w, r, &openaiReq)
		return
	}

	resp, usedProvider, err := s.engine.Execute(r.Context(), openaiReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	resp.Model = usedProvider.Model
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *server) handleChatStream(w http.ResponseWriter, r *http.Request, openaiReq *models.ChatCompletionRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return
	}
	fw := &flushingWriter{w: w, f: flusher}

	attempted := map[string]bool{}
	ctx := r.Context()
	committed := false

	for attempt := 0; attempt <= s.cfg.Routing.MaxRetries; attempt++ {
		p, err := s.engine.SelectProvider(attempted)
		if err != nil {
			break
		}
		start := time.Now()
		openaiReq.Stream = true
		ch, err := p.Client.ChatCompletionStream(ctx, *openaiReq)
		if err != nil {
			attempted[p.Name] = true
			if pe, ok := err.(*router.ProviderError); ok {
				s.engine.RecordFailure(p, pe.Retryable)
			} else {
				s.engine.RecordFailure(p, true)
			}
			continue
		}
		streamErr := writeStreamSSE(ch, p.Model, fw)
		latency := float64(time.Since(start).Microseconds()) / 1000.0
		if streamErr == nil {
			s.engine.RecordSuccess(p, latency)
		} else {
			s.engine.RecordFailure(p, true)
		}
		committed = true
		break
	}

	if !committed {
		fmt.Fprintf(fw, "data: {\"error\":\"all providers failed\"}\n\n")
	}
}

func (s *server) handleAggregatedChatNonStream(w http.ResponseWriter, r *http.Request, openaiReq *models.ChatCompletionRequest, aggAS *aggregationState) {
	ctx := r.Context()
	strat := router.Strategy(aggAS.Config.Strategy)
	candidates := s.buildAggCandidates(aggAS.Config.Models)
	remaining := candidates

	for attempt := 0; attempt < len(candidates); attempt++ {
		c, err := aggAS.selectCandidateLocked(remaining, strat)
		if err != nil {
			break
		}
		openaiReq.Model = c.Model
		start := time.Now()
		resp, err := c.Provider.Client.ChatCompletion(ctx, *openaiReq)
		latency := float64(time.Since(start).Microseconds()) / 1000.0

		if err == nil {
			s.engine.RecordSuccess(c.Provider, latency)
			resp.Model = c.Model
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		if pe, ok := err.(*router.ProviderError); ok {
			s.engine.RecordFailure(c.Provider, pe.Retryable)
		} else {
			s.engine.RecordFailure(c.Provider, true)
		}
		remaining = filterCandidates(remaining, c.Provider.Name)
	}
	writeError(w, http.StatusBadGateway, "api_error", "all aggregation entries failed")
}

func (s *server) handleAggregatedChatStream(w http.ResponseWriter, r *http.Request, openaiReq *models.ChatCompletionRequest, aggAS *aggregationState) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return
	}
	fw := &flushingWriter{w: w, f: flusher}

	ctx := r.Context()
	strat := router.Strategy(aggAS.Config.Strategy)
	candidates := s.buildAggCandidates(aggAS.Config.Models)
	remaining := candidates
	committed := false

	for attempt := 0; attempt < len(candidates); attempt++ {
		c, err := aggAS.selectCandidateLocked(remaining, strat)
		if err != nil {
			break
		}
		openaiReq.Model = c.Model
		start := time.Now()
		openaiReq.Stream = true
		ch, err := c.Provider.Client.ChatCompletionStream(ctx, *openaiReq)
		if err != nil {
			if pe, ok := err.(*router.ProviderError); ok {
				s.engine.RecordFailure(c.Provider, pe.Retryable)
			} else {
				s.engine.RecordFailure(c.Provider, true)
			}
			remaining = filterCandidates(remaining, c.Provider.Name)
			continue
		}
		streamErr := writeStreamSSE(ch, c.Model, fw)
		latency := float64(time.Since(start).Microseconds()) / 1000.0
		if streamErr == nil {
			s.engine.RecordSuccess(c.Provider, latency)
		} else {
			s.engine.RecordFailure(c.Provider, true)
		}
		committed = true
		break
	}

	if !committed {
		fmt.Fprintf(fw, "data: {\"error\":\"all providers failed\"}\n\n")
	}
}

// writeStreamSSE forwards OpenAI-style SSE chunks to the client. It returns the
// first error encountered while marshalling or writing a frame, so callers can
// avoid recording a false success when the stream fails mid-flight.
func writeStreamSSE(chunks <-chan models.ChatCompletionChunk, model string, w io.Writer) error {
	var firstErr error
	for chunk := range chunks {
		if chunk.Model == "" {
			chunk.Model = model
		}
		data, err := json.Marshal(chunk)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			// Writer is broken; no point continuing.
			return firstErr
		}
	}
	if _, err := fmt.Fprintf(w, "data: [DONE]\n\n"); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// ---- Helpers ----

type flushingWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw *flushingWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if fw.f != nil {
		fw.f.Flush()
	}
	return n, err
}

func writeError(w http.ResponseWriter, status int, etype, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    etype,
			"message": msg,
		},
	})
}

// resolveAutoModel queries the provider's /models endpoint and selects a
// suitable model, preferring one whose id contains "free". It sets
// pc.Model and returns the chosen model id.
func resolveAutoModel(pc *config.ProviderConfig) (string, error) {
	url := strings.TrimRight(pc.BaseURL, "/") + "/models"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if pc.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+pc.APIKey)
	}
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("models endpoint returned %d", resp.StatusCode)
	}
	var listing struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return "", err
	}
	if len(listing.Data) == 0 {
		return "", fmt.Errorf("no models returned")
	}
	chosen := listing.Data[0].ID
	for _, m := range listing.Data {
		if strings.Contains(strings.ToLower(m.ID), "free") {
			chosen = m.ID
			break
		}
	}
	pc.Model = chosen
	return chosen, nil
}

func estimateTokens(raw json.RawMessage) int {
	text := extractText(raw)
	// Rough approximation: ~4 characters per token.
	return len([]rune(text)) / 4
}

func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []models.ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func getBoolEnv(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func printUsage() {
	fmt.Println(`aimux — AI API Gateway with multi-provider routing

Usage:
  aimux setup               Interactive configuration TUI
  aimux server              Start the API gateway server
  aimux login <provider>    Authenticate with an AI provider
  aimux version             Show version
  aimux help                Show this help

Providers:
  chatgpt    Login via ChatGPT OAuth (for Codex models)`)

	fmt.Println(`
Examples:
  aimux setup               # First-time configuration wizard
  aimux server              # Start the gateway
  aimux login chatgpt       # Login with ChatGPT account

Server config:
  Set env vars or config.yaml. See README.md for details.`)
}
