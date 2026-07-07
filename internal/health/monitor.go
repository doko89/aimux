package health

import (
	"context"
	"log"
	"time"

	"ai-router/internal/router"
)

// Monitor periodically probes providers and updates circuit breakers.
type Monitor struct {
	engine           *router.Engine
	checkInterval    time.Duration
	failureThreshold int
	recoveryTimeout  time.Duration
	onStateChange    func(name string, from, to router.CircuitState)
	cancel           context.CancelFunc
}

// NewMonitor creates a health monitor.
func NewMonitor(engine *router.Engine, checkInterval, recoveryTimeout time.Duration, failureThreshold int, onStateChange func(string, router.CircuitState, router.CircuitState)) *Monitor {
	if checkInterval <= 0 {
		checkInterval = 30 * time.Second
	}
	if recoveryTimeout <= 0 {
		recoveryTimeout = 60 * time.Second
	}
	if failureThreshold <= 0 {
		failureThreshold = 5
	}
	return &Monitor{
		engine:           engine,
		checkInterval:    checkInterval,
		failureThreshold: failureThreshold,
		recoveryTimeout:  recoveryTimeout,
		onStateChange:    onStateChange,
	}
}

// Start launches the background probe loop.
func (m *Monitor) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.loop(ctx)
}

// Stop terminates the probe loop.
func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *Monitor) loop(ctx context.Context) {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkAll(ctx)
		}
	}
}

func (m *Monitor) checkAll(ctx context.Context) {
	for _, p := range m.engine.Providers() {
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		healthy := p.Client.HealthCheck(probeCtx)
		cancel()

		before := p.CircuitState()
		if healthy {
			p.HealthHealthy()
		} else {
			p.HealthUnhealthy(m.failureThreshold)
		}
		after := p.CircuitState()
		if before != after && m.onStateChange != nil {
			m.onStateChange(p.Name, before, after)
		}
		if !healthy {
			log.Printf("[health] provider %s unhealthy (state=%s)", p.Name, after)
		}
	}
}
