package setup

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

func mkInput(val, placeholder string) textinput.Model {
	t := textinput.New()
	t.SetValue(val)
	t.Placeholder = placeholder
	return t
}

type settingsPageModel struct {
	cfg        *SetupConfig
	page       int // 0=gateway, 1=circuit, 2=ratelimit, 3=auth
	focus      int
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
	authKeys   []string
	authNew    textinput.Model
	authEdit   bool
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
	m.authNew = mkInput("", "client-key-xxx")
	m.authKeys = make([]string, len(cfg.Auth.ValidAPIKeys))
	copy(m.authKeys, cfg.Auth.ValidAPIKeys)
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

	// Route typing to active input first (for nav keys, this is harmless)
	m.routeInput(keyMsg, key)

	switch key {
	case "tab":
		n := m.fieldCount()
		if m.focus < n-1 {
			m.focus++
			m.focusCurrent()
		} else if m.page < 3 {
			m.savePage()
			m.page++
			m.focus = 0
			m.focusCurrent()
		}
	case "enter":
		if m.page == 3 && m.authEdit {
			if v := m.authNew.Value(); v != "" {
				m.authKeys = append(m.authKeys, v)
			}
			m.authEdit = false
			m.focus = 0
		} else {
			m.savePage()
			if m.page < 3 {
				m.page++
				m.focus = 0
				m.focusCurrent()
			}
		}
	case " ":
		if m.page == 2 && m.focus == 0 {
			m.rlEnabled = !m.rlEnabled
		}
	case "a":
		if m.page == 3 && !m.authEdit {
			m.authEdit = true
			m.authNew.SetValue("")
			m.focus = 1
			m.focusCurrent()
		}
	case "d", "x", "delete":
		if m.page == 3 && !m.authEdit && len(m.authKeys) > 0 && m.focus < len(m.authKeys) {
			m.authKeys = append(m.authKeys[:m.focus], m.authKeys[m.focus+1:]...)
		}
	}

	return m, nil
}

func (m *settingsPageModel) routeInput(msg tea.KeyMsg, key string) {
	// navKeys — don't route these to inputs
	navKeys := map[string]bool{
		"tab": true, "enter": true, "esc": true, "escape": true,
		"up": true, "k": true, "down": true, "j": true,
		" ": true, "a": true, "d": true, "x": true, "delete": true,
	}
	if navKeys[key] {
		return
	}
	switch m.page {
	case 0:
		switch m.focus {
		case 0: m.host, _ = m.host.Update(msg)
		case 1: m.port, _ = m.port.Update(msg)
		case 2: m.strategy, _ = m.strategy.Update(msg)
		case 3: m.maxRetry, _ = m.maxRetry.Update(msg)
		}
	case 1:
		switch m.focus {
		case 0: m.cbThresh, _ = m.cbThresh.Update(msg)
		case 1: m.cbCooldown, _ = m.cbCooldown.Update(msg)
		case 2: m.cbHealth, _ = m.cbHealth.Update(msg)
		}
	case 2:
		switch m.focus {
		case 1: m.rlRPM, _ = m.rlRPM.Update(msg)
		case 2: m.rlBurst, _ = m.rlBurst.Update(msg)
		}
	case 3:
		if m.authEdit && m.focus == 1 {
			m.authNew, _ = m.authNew.Update(msg)
		}
	}
}

func (m *settingsPageModel) focusCurrent() {
	m.blurAll()
	switch m.page {
	case 0:
		switch m.focus {
		case 0: m.host.Focus()
		case 1: m.port.Focus()
		case 2: m.strategy.Focus()
		case 3: m.maxRetry.Focus()
		}
	case 1:
		switch m.focus {
		case 0: m.cbThresh.Focus()
		case 1: m.cbCooldown.Focus()
		case 2: m.cbHealth.Focus()
		}
	case 2:
		switch m.focus {
		case 1: m.rlRPM.Focus()
		case 2: m.rlBurst.Focus()
		}
	case 3:
		if m.authEdit && m.focus == 1 {
			m.authNew.Focus()
		}
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
	m.authNew.Blur()
}

func (m settingsPageModel) isEditing() bool {
	return true // always accept input
}

func (m settingsPageModel) fieldCount() int {
	switch m.page {
	case 0: return 4
	case 1: return 3
	case 2: return 3
	case 3: return 1 + len(m.authKeys)
	}
	return 0
}

func (m *settingsPageModel) savePage() {
	switch m.page {
	case 0:
		m.cfg.Gateway.Host = m.host.Value()
		if v, err := strconv.Atoi(m.port.Value()); err == nil {
			m.cfg.Gateway.Port = v
		}
		m.cfg.Routing.Strategy = m.strategy.Value()
		if v, err := strconv.Atoi(m.maxRetry.Value()); err == nil {
			m.cfg.Routing.MaxRetries = v
		}
	case 1:
		if v, err := strconv.Atoi(m.cbThresh.Value()); err == nil {
			m.cfg.CircuitBreaker.FailureThreshold = v
		}
		if v, err := strconv.ParseFloat(m.cbCooldown.Value(), 64); err == nil {
			m.cfg.CircuitBreaker.CooldownSeconds = v
		}
		if v, err := strconv.ParseFloat(m.cbHealth.Value(), 64); err == nil {
			m.cfg.CircuitBreaker.HealthCheckInterval = v
		}
	case 2:
		m.cfg.RateLimit.Enabled = m.rlEnabled
		if v, err := strconv.Atoi(m.rlRPM.Value()); err == nil {
			m.cfg.RateLimit.RPM = v
		}
		if v, err := strconv.Atoi(m.rlBurst.Value()); err == nil {
			m.cfg.RateLimit.Burst = v
		}
	case 3:
		m.cfg.Auth.ValidAPIKeys = m.authKeys
	}
}

func (m settingsPageModel) View() string {
	s := "\n"
	titles := []string{"Gateway/Routing", "Circuit Breaker", "Rate Limiting", "Auth Keys"}
	for i, t := range titles {
		mark := "○"
		style := normalStyle
		if m.page == i {
			mark = "●"
			style = activeTabStyle
		}
		s += fmt.Sprintf("  %s %s  ", mark, style.Render(t))
	}
	s += "\n\n"

	switch m.page {
	case 0:
		s += helpText.Render("── Gateway & Routing ──") + "\n\n"
		s += m.fieldRow("Host", m.host.View(), 0)
		s += m.fieldRow("Port", m.port.View(), 1)
		s += m.fieldRow("Strategy", m.strategy.View(), 2)
		s += m.fieldRow("Max Retry", m.maxRetry.View(), 3)
	case 1:
		s += helpText.Render("── Circuit Breaker ──") + "\n\n"
		s += m.fieldRow("Threshold", m.cbThresh.View(), 0)
		s += m.fieldRow("Cooldown(s)", m.cbCooldown.View(), 1)
		s += m.fieldRow("Health(s)", m.cbHealth.View(), 2)
	case 2:
		s += helpText.Render("── Rate Limiting ──") + "\n\n"
		enStr := "off"
		if m.rlEnabled { enStr = "on" }
		s += m.fieldRow("Enabled", enStr+" (space)", 0)
		s += m.fieldRow("RPM", m.rlRPM.View(), 1)
		s += m.fieldRow("Burst", m.rlBurst.View(), 2)
	case 3:
		s += helpText.Render("── Auth Keys ──") + "\n\n"
		if len(m.authKeys) == 0 {
			s += "  (none — all clients allowed)\n"
		} else {
			for i, k := range m.authKeys {
				cursor := "  "
				if !m.authEdit && m.focus == i {
					cursor = "▸ "
				}
				display := k
				if len(display) > 22 {
					display = display[:20] + "..."
				}
				s += cursor + normalStyle.Render(display) + "\n"
			}
		}
		if m.authEdit && m.focus == 1 {
			s += "\n" + m.fieldRow("New key", m.authNew.View(), 1)
		}
		s += "\n" + helpText.Render("a: add | d/x: remove")
	}

	s += "\n" + helpText.Render("tab: next | enter: save page")
	return s
}

func (m settingsPageModel) fieldRow(label, val string, focusIdx int) string {
	cursor := "  "
	if m.focus == focusIdx {
		cursor = "▸ "
	}
	return cursor + inputLabelStyle.Render(label) + val + "\n"
}
