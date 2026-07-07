package router

import (
	"fmt"
	"math/rand"
	"time"
)

// Candidate represents a (provider, model, weight) entry for aggregation routing.
type Candidate struct {
	Provider *Provider
	Model    string
	Weight   int
}

// SelectCandidate picks a candidate from the list using the given strategy.
// roundRobinIndex is a pointer to a caller-held counter for round-robin.
func SelectCandidate(candidates []Candidate, strategy Strategy, roundRobinIndex *int) (*Candidate, error) {
	available := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		ok := false
		c.Provider.mu.Lock()
		switch c.Provider.circuitState {
		case StateClosed, StateHalfOpen:
			ok = true
		case StateOpen:
			if time.Since(c.Provider.lastFailureTime).Seconds() > c.Provider.CooldownSeconds {
				c.Provider.circuitState = StateHalfOpen
				ok = true
			}
		}
		c.Provider.mu.Unlock()
		if ok {
			available = append(available, c)
		}
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("no available candidate in aggregation")
	}

	switch strategy {
	case StrategyRoundRobin:
		if roundRobinIndex == nil {
			return &available[0], nil
		}
		idx := *roundRobinIndex % len(available)
		*roundRobinIndex++
		return &available[idx], nil

	case StrategyWeighted:
		return weightedSelect(available), nil

	case StrategyFallback:
		return &available[0], nil

	default:
		return &available[0], nil
	}
}

func weightedSelect(candidates []Candidate) *Candidate {
	total := 0
	for _, c := range candidates {
		w := c.Weight
		if w <= 0 {
			w = 1
		}
		total += w
	}
	if total <= 0 {
		return &candidates[rand.Intn(len(candidates))]
	}
	r := rand.Intn(total)
	cum := 0
	for i := range candidates {
		w := candidates[i].Weight
		if w <= 0 {
			w = 1
		}
		cum += w
		if r < cum {
			return &candidates[i]
		}
	}
	return &candidates[len(candidates)-1]
}
