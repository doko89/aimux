package router

import (
	"ai-router/internal/models"
	"context"
	"errors"
	"testing"
)

type fakeClient struct {
	model   string
	failN   int
	calls   int
	healthy bool
}

func (f *fakeClient) ChatCompletion(ctx context.Context, req models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	f.calls++
	if f.calls <= f.failN {
		return nil, NewProviderError(503, "server", true, errors.New("boom"))
	}
	return &models.ChatCompletionResponse{
		Choices: []struct {
			Index        int                `json:"index"`
			Message      models.ChatMessage `json:"message"`
			FinishReason string             `json:"finish_reason"`
		}{
			{Message: models.ChatMessage{Role: "assistant", Content: "ok"}, FinishReason: "stop"},
		},
		Usage: models.OAUsage{PromptTokens: 5, CompletionTokens: 3},
	}, nil
}

func (f *fakeClient) ChatCompletionStream(ctx context.Context, req models.ChatCompletionRequest) (<-chan models.ChatCompletionChunk, error) {
	ch := make(chan models.ChatCompletionChunk, 1)
	close(ch)
	return ch, nil
}

func (f *fakeClient) HealthCheck(ctx context.Context) bool { return f.healthy }

func newTestProviders() []*Provider {
	return []*Provider{
		{Name: "a", Model: "gpt-4o", Weight: 1, Priority: 1, FailureThreshold: 2, CooldownSeconds: 0, Client: &fakeClient{}},
		{Name: "b", Model: "gpt-4o", Weight: 1, Priority: 2, FailureThreshold: 2, CooldownSeconds: 0, Client: &fakeClient{}},
	}
}

func TestStrategyRoundRobin(t *testing.T) {
	ps := newTestProviders()
	e, err := NewEngine(ps, StrategyRoundRobin, 2)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for i := 0; i < 4; i++ {
		p, err := e.SelectProvider(nil)
		if err != nil {
			t.Fatal(err)
		}
		seen[p.Name]++
	}
	if seen["a"] != 2 || seen["b"] != 2 {
		t.Errorf("expected even distribution, got %v", seen)
	}
}

func TestStrategyFallback(t *testing.T) {
	ps := newTestProviders()
	e, err := NewEngine(ps, StrategyFallback, 2)
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.SelectProvider(nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "a" {
		t.Errorf("expected priority a, got %s", p.Name)
	}
}

func TestExecuteWithFallback(t *testing.T) {
	ps := newTestProviders()
	// Make provider a fail its first call, then succeed.
	ps[0].Client = &fakeClient{failN: 1}
	e, err := NewEngine(ps, StrategyFallback, 2)
	if err != nil {
		t.Fatal(err)
	}
	resp, used, err := e.Execute(context.Background(), models.ChatCompletionRequest{Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	if used.Name != "b" {
		t.Errorf("expected fallback to b, got %s", used.Name)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
}
