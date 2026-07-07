package setup

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type settingsPageModel struct {
	cfg        *SetupConfig
	focus      int // 0-3=gateway, 4-6=circuit, 7=ratelimit toggle, 8-9=ratelimit
	host       textinput.Model
	port       textinput.Model
	strategy   textinput.Model
	maxRetry   textinput.Model
	cbThresh   textinput.Model
	cbCooldown textinput.Model
	cbHealth   textinput.Model
	rlEnabled  bool
	rlRPM      textinput.Model
	rlBurst    textinput.Model
}

func newSettingsPage(cfg *SetupConfig) settingsPageModel {
	m := settingsPageModel{
		cfg:       cfg,
		rlEnabled: cfg.RateLimit.Enabled,
	}
	m.host = mkInput(cfg.Gateway.Host, "0.0.0.0")
	m.port = mkInput(strconv.Itoa(cfg.Gateway.Port), "8080")
	m.strategy = mkInput(cfg.Routing.Strategy, "weighted")
	m.maxRetry = mkInput(strconv.Itoa(cfg.Routing.MaxRetries), "2")
	m.cbThresh = mkInput(strconv.Itoa(cfg.CircuitBreaker.FailureThreshold), "5")
	m.cbCooldown = mkInput(fmt.Sprintf("%.0f", cfg.CircuitBreaker.CooldownSeconds), "60")
	m.cbHealth = mkInput(fmt.Sprintf("%.0f", cfg.CircuitBreaker.HealthCheckInterval), "30")
	m.rlRPM = mkInput(strconv.Itoa(cfg.RateLimit.RPM), "100")
	m.rlBurst = mkInput(strconv.Itoa(cfg.RateLimit.Burst), "20")
	m.host.Focus()
	return m
}

func (m settingsPageModel) Init() tea.Cmd { return textinput.Blink }

func (m settingsPageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	new, cmd := m.update(msg, m.cfg)
	return new, cmd
}

func (m settingsPageModel) update(msg tea.Msg, cfg *SetupConfig) (settingsPageModel, tea.Cmd) {
	m.cfg = cfg

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	key := keyMsg.String()

	// Nav keys — handle first
	switch key {
	case "up", "k":
		if m.focus > 0 {
			m.focus--
			m.blurAll()
		}
		return m, nil
	case "down", "j":
		if m.focus < 9 {
			m.focus++
			m.blurAll()
		}
		return m, nil
	case "escape":
		m.savePage()
		return m, nil
	case " ":
		if m.focus == 7 {
			m.rlEnabled = !m.rlEnabled
		}
		return m, nil
	case "enter":
		m.savePage()
		return m, nil
	}

	// Typing → route to active textinput
	m.routeInput(keyMsg)
	return m, nil
}

func (m *settingsPageModel) routeInput(msg tea.KeyMsg) {
	switch m.focus {
	case 0: m.host, _ = m.host.Update(msg)
	case 1: m.port, _ = m.port.Update(msg)
	case 2: m.strategy, _ = m.strategy.Update(msg)
	case 3: m.maxRetry, _ = m.maxRetry.Update(msg)
	case 4: m.cbThresh, _ = m.cbThresh.Update(msg)
	case 5: m.cbCooldown, _ = m.cbCooldown.Update(msg)
	case 6: m.cbHealth, _ = m.cbHealth.Update(msg)
	// 7 = toggle (space handles it)
	case 8: m.rlRPM, _ = m.rlRPM.Update(msg)
	case 9: m.rlBurst, _ = m.rlBurst.Update(msg)
	}
}

func (m *settingsPageModel) blurAll() {
	m.host.Blur()
	m.port.Blur()
	m.strategy.Blur()
	m.maxRetry.Blur()
	m.cbThresh.Blur()
	m.cbCooldown.Blur()
	m.cbHealth.Blur()
	m.rlRPM.Blur()
	m.rlBurst.Blur()
	switch m.focus {
	case 0: m.host.Focus()
	case 1: m.port.Focus()
	case 2: m.strategy.Focus()
	case 3: m.maxRetry.Focus()
	case 4: m.cbThresh.Focus()
	case 5: m.cbCooldown.Focus()
	case 6: m.cbHealth.Focus()
	case 8: m.rlRPM.Focus()
	case 9: m.rlBurst.Focus()
	}
}

func (m settingsPageModel) isEditing() bool { return true }

func (m *settingsPageModel) savePage() {
	if v, err := strconv.Atoi(m.port.Value()); err == nil {
		m.cfg.Gateway.Port = v
	}
	m.cfg.Gateway.Host = m.host.Value()
	m.cfg.Routing.Strategy = m.strategy.Value()
	if v, err := strconv.Atoi(m.maxRetry.Value()); err == nil {
		m.cfg.Routing.MaxRetries = v
	}
	if v, err := strconv.Atoi(m.cbThresh.Value()); err == nil {
		m.cfg.CircuitBreaker.FailureThreshold = v
	}
	if v, err := strconv.ParseFloat(m.cbCooldown.Value(), 64); err == nil {
		m.cfg.CircuitBreaker.CooldownSeconds = v
	}
	if v, err := strconv.ParseFloat(m.cbHealth.Value(), 64); err == nil {
		m.cfg.CircuitBreaker.HealthCheckInterval = v
	}
	m.cfg.RateLimit.Enabled = m.rlEnabled
	if v, err := strconv.Atoi(m.rlRPM.Value()); err == nil {
		m.cfg.RateLimit.RPM = v
	}
	if v, err := strconv.Atoi(m.rlBurst.Value()); err == nil {
		m.cfg.RateLimit.Burst = v
	}
}

func (m settingsPageModel) View() string {
	s := "\n"

	// Gateway & Routing
	s += helpText.Render("Gateway & Routing") + "\n\n"
	s += m.fieldRow("Host", m.host.View(), 0)
	s += m.fieldRow("Port", m.port.View(), 1)
	s += m.fieldRow("Strategy", m.strategy.View(), 2)
	s += m.fieldRow("Max Retry", m.maxRetry.View(), 3)

	s += "\n"

	// Circuit Breaker
	s += helpText.Render("Circuit Breaker") + "\n\n"
	s += m.fieldRow("Threshold", m.cbThresh.View(), 4)
	s += m.fieldRow("Cooldown(s)", m.cbCooldown.View(), 5)
	s += m.fieldRow("Health(s)", m.cbHealth.View(), 6)

	s += "\n"

	// Rate Limiting
	s += helpText.Render("Rate Limiting") + "\n\n"
	enStr := "off"
	if m.rlEnabled {
		enStr = "on"
	}
	s += m.fieldRow("Enabled", enStr+" (space)", 7)
	s += m.fieldRow("RPM", m.rlRPM.View(), 8)
	s += m.fieldRow("Burst", m.rlBurst.View(), 9)

	s += "\n" + helpText.Render("↑↓: navigate | space: toggle | enter: save")
	return s
}

func (m settingsPageModel) fieldRow(label, val string, focusIdx int) string {
	cursor := "  "
	if m.focus == focusIdx {
		cursor = "▸ "
	}
	return cursor + inputLabelStyle.Render(label) + val + "\n"
}
