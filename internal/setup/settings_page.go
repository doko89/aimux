package setup

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

// ─── Settings Page: everything else ───────────────────────────────

type settingsPageModel struct {
	cfg   *SetupConfig
	focus int
	pages [5]textinput.Model // host, port, strategy, maxRetries, circuit breaker threshold
	page  int                // 0=gateway, 1=routing, 2=circuit, 3=ratelimit, 4=auth
	// ratelimit
	rlEnabled bool
	rlRPM     textinput.Model
	rlBurst   textinput.Model
	// circuit breaker
	cbThresh  textinput.Model
	cbCooldown textinput.Model
	cbHealth  textinput.Model
	// auth
	authKeys []string
	authNew  textinput.Model
}

func newSettingsPage(cfg *SetupConfig) settingsPageModel {
	m := settingsPageModel{
		cfg:       cfg,
		rlEnabled: cfg.RateLimit.Enabled,
	}

	m.pages[0] = textinput.New()
	m.pages[0].SetValue(cfg.Gateway.Host)
	m.pages[1] = textinput.New()
	m.pages[1].SetValue(strconv.Itoa(cfg.Gateway.Port))
	m.pages[2] = textinput.New()
	m.pages[2].SetValue(cfg.Routing.Strategy)
	m.pages[3] = textinput.New()
	m.pages[3].SetValue(strconv.Itoa(cfg.Routing.MaxRetries))

	m.cbThresh = textinput.New()
	m.cbThresh.SetValue(strconv.Itoa(cfg.CircuitBreaker.FailureThreshold))
	m.cbCooldown = textinput.New()
	m.cbCooldown.SetValue(fmt.Sprintf("%.0f", cfg.CircuitBreaker.CooldownSeconds))
	m.cbHealth = textinput.New()
	m.cbHealth.SetValue(fmt.Sprintf("%.0f", cfg.CircuitBreaker.HealthCheckInterval))

	m.rlRPM = textinput.New()
	m.rlRPM.SetValue(strconv.Itoa(cfg.RateLimit.RPM))
	m.rlBurst = textinput.New()
	m.rlBurst.SetValue(strconv.Itoa(cfg.RateLimit.Burst))

	m.authNew = textinput.New()
	m.authNew.Placeholder = "client-key-xxx"
	m.authKeys = make([]string, len(cfg.Auth.ValidAPIKeys))
	copy(m.authKeys, cfg.Auth.ValidAPIKeys)

	m.pages[0].Focus()
	return m
}

func (m settingsPageModel) Init() tea.Cmd { return textinput.Blink }

func (m settingsPageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.focus++
			m.focus %= 4
		case " ":
			if m.page == 3 && m.focus == 0 {
				m.rlEnabled = !m.rlEnabled
			}
		case "enter":
			// Save current section and move to next
			m.saveCurrentPage()
			if m.page < 4 {
				m.page++
				m.focus = 0
			}
		case "up":
			if m.focus > 0 {
				m.focus--
			}
		case "down", "j":
			if m.focus < 3 {
				m.focus++
			}
		case "a":
			if m.page == 4 && m.focus == 0 {
				m.authNew.SetValue("")
				m.focus = 1
			}
		case "d", "delete":
			if m.page == 4 && m.focus == 0 && m.authKeys != nil && len(m.authKeys) > 0 && m.focus < len(m.authKeys) {
				idx := m.focus
				m.authKeys = append(m.authKeys[:idx], m.authKeys[idx+1:]...)
			}
		}
	}

	// Route input updates
	if m.page == 0 && m.focus < 4 {
		var cmd tea.Cmd
		m.pages[m.focus], cmd = m.pages[m.focus].Update(msg)
		return m, cmd
	}
	if m.page == 1 && m.focus == 0 {
		var cmd tea.Cmd
		m.pages[2], cmd = m.pages[2].Update(msg)
		return m, cmd
	}
	if m.page == 1 && m.focus == 1 {
		var cmd tea.Cmd
		m.pages[3], cmd = m.pages[3].Update(msg)
		return m, cmd
	}
	if m.page == 2 {
		inputs := []*textinput.Model{&m.cbThresh, &m.cbCooldown, &m.cbHealth}
		if m.focus < 3 {
			var cmd tea.Cmd
			*inputs[m.focus], cmd = inputs[m.focus].Update(msg)
			return m, cmd
		}
	}
	if m.page == 3 {
		inputs := []*textinput.Model{&m.rlRPM, &m.rlBurst}
		if m.focus >= 1 && m.focus-1 < 2 {
			var cmd tea.Cmd
			*inputs[m.focus-1], cmd = inputs[m.focus-1].Update(msg)
			return m, cmd
		}
	}
	if m.page == 4 && m.focus == 1 {
		var cmd tea.Cmd
		m.authNew, cmd = m.authNew.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *settingsPageModel) saveCurrentPage() {
	switch m.page {
	case 0:
		m.cfg.Gateway.Host = m.pages[0].Value()
		if v, err := strconv.Atoi(m.pages[1].Value()); err == nil {
			m.cfg.Gateway.Port = v
		}
	case 1:
		m.cfg.Routing.Strategy = m.pages[2].Value()
		if v, err := strconv.Atoi(m.pages[3].Value()); err == nil {
			m.cfg.Routing.MaxRetries = v
		}
	case 2:
		if v, err := strconv.Atoi(m.cbThresh.Value()); err == nil {
			m.cfg.CircuitBreaker.FailureThreshold = v
		}
		if v, err := strconv.ParseFloat(m.cbCooldown.Value(), 64); err == nil {
			m.cfg.CircuitBreaker.CooldownSeconds = v
		}
		if v, err := strconv.ParseFloat(m.cbHealth.Value(), 64); err == nil {
			m.cfg.CircuitBreaker.HealthCheckInterval = v
		}
	case 3:
		m.cfg.RateLimit.Enabled = m.rlEnabled
		if v, err := strconv.Atoi(m.rlRPM.Value()); err == nil {
			m.cfg.RateLimit.RPM = v
		}
		if v, err := strconv.Atoi(m.rlBurst.Value()); err == nil {
			m.cfg.RateLimit.Burst = v
		}
	case 4:
		m.cfg.Auth.ValidAPIKeys = m.authKeys
	}
}

func (m settingsPageModel) View() string {
	s := "\n"

	titles := []string{"Gateway", "Routing", "Circuit Breaker", "Rate Limiting", "Auth"}
	for i, t := range titles {
		mark := "○"
		if m.page == i {
			mark = "●"
		}
		style := normalStyle
		if m.page == i {
			style = activeTabStyle
		}
		s += fmt.Sprintf("  %s ", style.Render(mark)) + style.Render(t) + "  "
	}
	s += "\n\n"

	switch m.page {
	case 0:
		s += m.renderGateway()
	case 1:
		s += m.renderRouting()
	case 2:
		s += m.renderCircuit()
	case 3:
		s += m.renderRateLimit()
	case 4:
		s += m.renderAuth()
	}

	s += "\n" + helpText.Render("enter: next page | tab: cycle fields | s: save all")
	return s
}

func (m settingsPageModel) renderGateway() string {
	labels := []string{"Host", "Port", "Strategy", "Max Retries"}
	s := helpText.Render("── Gateway & Routing ──") + "\n\n"
	for i, label := range labels {
		cursor := "  "
		if m.focus == i {
			cursor = "▸ "
		}
		s += cursor + inputLabelStyle.Render(label) + m.pages[i].View() + "\n"
	}
	return s
}

func (m settingsPageModel) renderRouting() string {
	s := helpText.Render("── Routing ──") + "\n\n"
	cursor := "  "
	if m.focus == 0 {
		cursor = "▸ "
	}
	s += cursor + inputLabelStyle.Render("Strategy") + m.pages[2].View() + "\n"
	cursor = "  "
	if m.focus == 1 {
		cursor = "▸ "
	}
	s += cursor + inputLabelStyle.Render("Max Retries") + m.pages[3].View() + "\n"
	s += "\n  " + normalStyle.Render("(leave Strategy empty for weighted)")
	return s
}

func (m settingsPageModel) renderCircuit() string {
	s := helpText.Render("── Circuit Breaker ──") + "\n\n"
	labels := []string{"Threshold", "Cooldown(s)", "Health(s)"}
	inputs := []*textinput.Model{&m.cbThresh, &m.cbCooldown, &m.cbHealth}
	for i, label := range labels {
		cursor := "  "
		if m.focus == i {
			cursor = "▸ "
		}
		s += cursor + inputLabelStyle.Render(label) + inputs[i].View() + "\n"
	}
	return s
}

func (m settingsPageModel) renderRateLimit() string {
	s := helpText.Render("── Rate Limiting ──") + "\n\n"
	cursor := "  "
	if m.focus == 0 {
		cursor = "▸ "
	}
	en := "off"
	if m.rlEnabled {
		en = "on"
	}
	s += cursor + inputLabelStyle.Render("Enabled") + en + " (space)" + "\n"
	cursor = "  "
	if m.focus == 1 {
		cursor = "▸ "
	}
	s += cursor + inputLabelStyle.Render("RPM") + m.rlRPM.View() + "\n"
	cursor = "  "
	if m.focus == 2 {
		cursor = "▸ "
	}
	s += cursor + inputLabelStyle.Render("Burst") + m.rlBurst.View() + "\n"
	return s
}

func (m settingsPageModel) renderAuth() string {
	s := helpText.Render("── Auth (API Keys) ──") + "\n\n"
	if len(m.authKeys) == 0 {
		s += "  (no keys — all clients allowed)\n"
	} else {
		for _, k := range m.authKeys {
			cursor := "  "
			if m.focus == 0 {
				cursor = "▸ "
			}
			display := k
			if len(display) > 25 {
				display = display[:22] + "..."
			}
			s += cursor + normalStyle.Render(display) + "\n"
		}
	}
	if m.focus == 1 {
		s += "\n  " + inputLabelStyle.Render("New") + m.authNew.View() + "\n"
	}
	s += "\n" + helpText.Render("a: add | d: remove")
	return s
}
