package setup

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type routingModel struct {
	cfg      *SetupConfig
	back     *mainMenuModel
	strategy int // 0=weighted, 1=fallback, 2=round_robin, 3=least_latency
	fallback bool
	retries  textinput.Model
	cursor   int
}

func newRoutingModel(cfg *SetupConfig, back *mainMenuModel) routingModel {
	r := textinput.New()
	r.SetValue(strconv.Itoa(cfg.Routing.MaxRetries))
	r.Focus()
	strat := 0
	strats := []string{"weighted", "fallback", "round_robin", "least_latency"}
	for i, s := range strats {
		if s == cfg.Routing.Strategy { strat = i; break }
	}
	return routingModel{cfg: cfg, back: back, strategy: strat, fallback: cfg.Routing.FallbackOnError, retries: r}
}

func (m routingModel) Init() tea.Cmd { return textinput.Blink }

func (m routingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc": return m.back, nil
		case "enter":
			strats := []string{"weighted", "fallback", "round_robin", "least_latency"}
			m.cfg.Routing.Strategy = strats[m.strategy]
			m.cfg.Routing.FallbackOnError = m.fallback
			if v, err := strconv.Atoi(m.retries.Value()); err == nil { m.cfg.Routing.MaxRetries = v }
			return m.back, nil
		case "up", "k":
			if m.cursor == 0 && m.strategy > 0 { m.strategy-- }
		case "down", "j":
			if m.cursor == 0 && m.strategy < 3 { m.strategy++ }
		case " ":
			if m.cursor == 1 { m.fallback = !m.fallback }
		case "tab":
			if m.cursor < 2 { m.cursor++ }
		}
	}
	if m.cursor == 2 {
		var cmd tea.Cmd
		m.retries, cmd = m.retries.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m routingModel) View() string {
	s := "\n  \033[1mRouting\033[0m\n\n"
	strats := []string{"weighted", "fallback", "round_robin", "least_latency"}
	for i, st := range strats {
		cursor := "  "
		if m.strategy == i { cursor = "\u25b6 " }
		s += cursor + st + "\n"
	}
	fbStr := "off"
	if m.fallback { fbStr = "on" }
	s += fmt.Sprintf("\n  Fallback on error: %s (space to toggle)\n", fbStr)
	s += "  Max retries:       " + m.retries.View() + "\n"
	s += "\n  enter: save | esc: back"
	return s
}
