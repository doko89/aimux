package router

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"ai-router/internal/models"
)

// CircuitState represents the circuit breaker state for a provider.
type CircuitState string

const (
	StateClosed   CircuitState = "closed"
	StateOpen     CircuitState = "open"
	StateHalfOpen CircuitState = "half_open"
)

// Strategy is the routing strategy identifier.
type Strategy string

const (
	StrategyRoundRobin   Strategy = "round_robin"
	StrategyFallback     Strategy = "fallback"
	StrategyWeighted     Strategy = "weighted"
	StrategyLeastLatency Strategy = "least_latency"
)

// ProviderClient is implemented by concrete provider adapters.
type ProviderClient interface {
	ChatCompletion(ctx context.Context, req models.ChatCompletionRequest) (*models.ChatCompletionResponse, error)
	ChatCompletionStream(ctx context.Context, req models.ChatCompletionRequest) (<-chan models.ChatCompletionChunk, error)
	HealthCheck(ctx context.Context) bool
}

// Provider holds runtime metadata + metrics for a single provider.
type Provider struct {
	Name             string
	BaseURL          string
	Model            string
	Weight           int
	Priority         int
	MaxRPM           int
	FailureThreshold int
	CooldownSeconds  float64
	Passthrough      bool
	Client           ProviderClient

	mu                  sync.Mutex
	circuitState        CircuitState
	consecutiveFailures int
	lastFailureTime     time.Time
	totalRequests       int
	totalFailures       int
	avgLatencyMs        float64
	lastUsedTime        time.Time
}

// Engine selects providers and orchestrates fallback.
type Engine struct {
	providers  map[string]*Provider
	order      []*Provider
	strategy   Strategy
	maxRetries int
	rrIndex    int
	mu         sync.Mutex
}

// NewEngine builds a routing engine from enabled providers.
func NewEngine(providers []*Provider, strategy Strategy, maxRetries int) (*Engine, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}
	pm := make(map[string]*Provider, len(providers))
	order := make([]*Provider, 0, len(providers))
	for _, p := range providers {
		p.circuitState = StateClosed
		if p.FailureThreshold == 0 {
			p.FailureThreshold = 5
		}
		if p.CooldownSeconds == 0 {
			p.CooldownSeconds = 60
		}
		pm[p.Name] = p
		order = append(order, p)
	}
	// Deterministic ordering by priority for fallback strategy.
	sort.SliceStable(order, func(i, j int) bool {
		return order[i].Priority < order[j].Priority
	})
	return &Engine{
		providers:  pm,
		order:      order,
		strategy:   strategy,
		maxRetries: maxRetries,
	}, nil
}

// Providers returns all configured providers (metadata access).
func (e *Engine) Providers() map[string]*Provider { return e.providers }

// Strategy returns the active routing strategy.
func (e *Engine) Strategy() Strategy { return e.strategy }

// SelectProvider picks a provider based on the configured strategy,
// excluding the named providers.
func (e *Engine) SelectProvider(exclude map[string]bool) (*Provider, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	available := make([]*Provider, 0, len(e.order))
	for _, p := range e.order {
		if exclude != nil && exclude[p.Name] {
			continue
		}
		if e.isAvailable(p) {
			available = append(available, p)
		}
	}

	if len(available) == 0 {
		// Allow half-open probes as last resort.
		for _, p := range e.order {
			if exclude != nil && exclude[p.Name] {
				continue
			}
			if p.circuitState == StateHalfOpen {
				available = append(available, p)
			}
		}
	}

	if len(available) == 0 {
		return nil, fmt.Errorf("no provider available")
	}

	switch e.strategy {
	case StrategyFallback:
		return e.fallback(available), nil
	case StrategyWeighted:
		return e.weighted(available), nil
	case StrategyLeastLatency:
		return e.leastLatency(available), nil
	case StrategyRoundRobin:
		return e.roundRobin(available), nil
	default:
		return available[0], nil
	}
}

func (e *Engine) roundRobin(available []*Provider) *Provider {
	p := available[e.rrIndex%len(available)]
	e.rrIndex++
	return p
}

func (e *Engine) fallback(available []*Provider) *Provider {
	best := available[0]
	for _, p := range available[1:] {
		if p.Priority < best.Priority {
			best = p
		}
	}
	return best
}

func (e *Engine) weighted(available []*Provider) *Provider {
	total := 0
	for _, p := range available {
		total += p.Weight
	}
	if total <= 0 {
		return available[rand.Intn(len(available))]
	}
	r := rand.Intn(total)
	cum := 0
	for _, p := range available {
		cum += p.Weight
		if r < cum {
			return p
		}
	}
	return available[len(available)-1]
}

func (e *Engine) leastLatency(available []*Provider) *Provider {
	best := available[0]
	for _, p := range available[1:] {
		if p.avgLatencyMs < best.avgLatencyMs {
			best = p
		}
	}
	return best
}

// RecordSuccess updates metrics and (possibly) closes the circuit.
func (e *Engine) RecordSuccess(p *Provider, latencyMs float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.totalRequests++
	p.consecutiveFailures = 0
	p.lastUsedTime = time.Now()
	alpha := 0.3
	if p.avgLatencyMs == 0 {
		p.avgLatencyMs = latencyMs
	} else {
		p.avgLatencyMs = alpha*latencyMs + (1-alpha)*p.avgLatencyMs
	}
	if p.circuitState == StateHalfOpen {
		p.circuitState = StateClosed
	}
}

// RecordFailure records a failure and trips the breaker if needed.
func (e *Engine) RecordFailure(p *Provider, retryable bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.totalRequests++
	if retryable {
		p.totalFailures++
		p.consecutiveFailures++
		p.lastFailureTime = time.Now()
		if p.consecutiveFailures >= p.FailureThreshold {
			p.circuitState = StateOpen
		}
	}
}

// isAvailable checks the circuit breaker state for a provider.
func (e *Engine) isAvailable(p *Provider) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch p.circuitState {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(p.lastFailureTime).Seconds() > p.CooldownSeconds {
			p.circuitState = StateHalfOpen
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// CircuitState returns the current breaker state (copy-safe).
func (p *Provider) CircuitState() CircuitState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.circuitState
}

// Snapshot returns a point-in-time copy of metrics for health reporting.
type ProviderSnapshot struct {
	Name          string
	Model         string
	CircuitState  CircuitState
	AvgLatencyMs  float64
	TotalRequests int
	TotalFailures int
	Weight        int
	Priority      int
}

// HealthHealthy records a successful health probe.
func (p *Provider) HealthHealthy() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveFailures = 0
	if p.circuitState == StateHalfOpen {
		p.circuitState = StateClosed
	}
}

// HealthUnhealthy records a failed health probe and trips the breaker.
func (p *Provider) HealthUnhealthy(threshold int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveFailures++
	p.lastFailureTime = time.Now()
	if p.consecutiveFailures >= threshold && p.circuitState == StateClosed {
		p.circuitState = StateOpen
	}
}

// Snapshot returns provider metrics.
func (p *Provider) Snapshot() ProviderSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	fr := 0.0
	if p.totalRequests > 0 {
		fr = float64(p.totalFailures) / float64(p.totalRequests) * 100
	}
	_ = fr
	return ProviderSnapshot{
		Name:          p.Name,
		Model:         p.Model,
		CircuitState:  p.circuitState,
		AvgLatencyMs:  p.avgLatencyMs,
		TotalRequests: p.totalRequests,
		TotalFailures: p.totalFailures,
		Weight:        p.Weight,
		Priority:      p.Priority,
	}
}

// Execute runs a non-streaming request with fallback across providers.
func (e *Engine) Execute(
	ctx context.Context,
	req models.ChatCompletionRequest,
) (*models.ChatCompletionResponse, *Provider, error) {
	attempted := map[string]bool{}
	var lastErr error

	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		p, err := e.SelectProvider(attempted)
		if err != nil {
			lastErr = err
			break
		}
		start := time.Now()
		resp, perr := p.Client.ChatCompletion(ctx, req)
		latency := float64(time.Since(start).Microseconds()) / 1000.0

		if perr == nil {
			e.RecordSuccess(p, latency)
			return resp, p, nil
		}

		lastErr = perr
		attempted[p.Name] = true
		if pe, ok := perr.(*ProviderError); ok {
			e.RecordFailure(p, pe.Retryable)
		} else {
			e.RecordFailure(p, true)
		}
	}
	return nil, nil, fmt.Errorf("all providers failed: %w", lastErr)
}

// ProviderError classifies a provider failure.
type ProviderError struct {
	Status    int
	Category  string // rate_limit | server | timeout | client
	Retryable bool
	Err       error
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("provider error (status=%d, category=%s): %v", e.Status, e.Category, e.Err)
	}
	return fmt.Sprintf("provider error (status=%d, category=%s)", e.Status, e.Category)
}

func (e *ProviderError) Unwrap() error { return e.Err }

// NewProviderError builds a classified provider error.
func NewProviderError(status int, category string, retryable bool, err error) *ProviderError {
	return &ProviderError{Status: status, Category: category, Retryable: retryable, Err: err}
}
