package setup

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type circuitBreakerModel struct {
	cfg      *SetupConfig
	back     *mainMenuModel
	thresh   textinput.Model
	cooldown textinput.Model
	health   textinput.Model
	cursor   int
}

func newCircuitBreakerModel(cfg *SetupConfig, back *mainMenuModel) circuitBreakerModel {
	th := textinput.New()
	th.SetValue(strconv.Itoa(cfg.CircuitBreaker.FailureThreshold))
	th.Focus()
	cd := textinput.New()
	cd.SetValue(fmt.Sprintf("%.0f", cfg.CircuitBreaker.CooldownSeconds))
	hl := textinput.New()
	hl.SetValue(fmt.Sprintf("%.0f", cfg.CircuitBreaker.HealthCheckInterval))
	return circuitBreakerModel{cfg: cfg, back: back, thresh: th, cooldown: cd, health: hl}
}

func (m circuitBreakerModel) Init() tea.Cmd { return textinput.Blink }

func (m circuitBreakerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc": return m.back, nil
		case "enter":
			if v, err := strconv.Atoi(m.thresh.Value()); err == nil { m.cfg.CircuitBreaker.FailureThreshold = v }
			if v, err := strconv.ParseFloat(m.cooldown.Value(), 64); err == nil { m.cfg.CircuitBreaker.CooldownSeconds = v }
			if v, err := strconv.ParseFloat(m.health.Value(), 64); err == nil { m.cfg.CircuitBreaker.HealthCheckInterval = v }
			return m.back, nil
		case "tab", "down":
			if m.cursor < 2 { m.cursor++ }
		case "up":
			if m.cursor > 0 { m.cursor-- }
		}
	}
	inputs := []textinput.Model{m.thresh, m.cooldown, m.health}
	if m.cursor < len(inputs) {
		var cmd tea.Cmd
		inputs[m.cursor], cmd = inputs[m.cursor].Update(msg)
		if m.cursor == 0 { m.thresh = inputs[0] }
		if m.cursor == 1 { m.cooldown = inputs[1] }
		if m.cursor == 2 { m.health = inputs[2] }
		return m, cmd
	}
	return m, nil
}

func (m circuitBreakerModel) View() string {
	s := "\n  \033[1mCircuit Breaker\033[0m\n\n"
	s += "  Failure threshold:     " + m.thresh.View() + "\n"
	s += "  Cooldown (seconds):    " + m.cooldown.View() + "\n"
	s += "  Health check interval: " + m.health.View() + "\n"
	s += "\n  enter: save | esc: back"
	return s
}
