package setup

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type rateLimitModel struct {
	cfg      *SetupConfig
	back     *mainMenuModel
	enabled  bool
	rpm      textinput.Model
	burst    textinput.Model
	cursor   int
}

func newRateLimitModel(cfg *SetupConfig, back *mainMenuModel) rateLimitModel {
	r := textinput.New()
	r.SetValue(strconv.Itoa(cfg.RateLimit.RPM))
	r.Focus()
	b := textinput.New()
	b.SetValue(strconv.Itoa(cfg.RateLimit.Burst))
	return rateLimitModel{cfg: cfg, back: back, enabled: cfg.RateLimit.Enabled, rpm: r, burst: b}
}

func (m rateLimitModel) Init() tea.Cmd { return textinput.Blink }

func (m rateLimitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc": return m.back, nil
		case "enter":
			m.cfg.RateLimit.Enabled = m.enabled
			if v, err := strconv.Atoi(m.rpm.Value()); err == nil { m.cfg.RateLimit.RPM = v }
			if v, err := strconv.Atoi(m.burst.Value()); err == nil { m.cfg.RateLimit.Burst = v }
			return m.back, nil
		case " ":
			if m.cursor == 0 { m.enabled = !m.enabled }
		case "tab", "down":
			if m.cursor < 2 { m.cursor++ }
		case "up":
			if m.cursor > 0 { m.cursor-- }
		}
	}
	if m.cursor == 1 {
		var cmd tea.Cmd
		m.rpm, cmd = m.rpm.Update(msg)
		return m, cmd
	}
	if m.cursor == 2 {
		var cmd tea.Cmd
		m.burst, cmd = m.burst.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m rateLimitModel) View() string {
	s := "\n  \033[1mRate Limiting\033[0m\n\n"
	enStr := "off"
	if m.enabled { enStr = "on" }
	s += fmt.Sprintf("  Enabled: %s (space to toggle)\n", enStr)
	s += "  RPM:     " + m.rpm.View() + "\n"
	s += "  Burst:   " + m.burst.View() + "\n"
	s += "\n  enter: save | esc: back"
	return s
}
