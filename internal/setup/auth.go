package setup

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type authModel struct {
	cfg    *SetupConfig
	back   *mainMenuModel
	keys   []string
	newKey textinput.Model
	cursor int
	phase  string // "list", "add"
}

func newAuthModel(cfg *SetupConfig, back *mainMenuModel) authModel {
	n := textinput.New()
	n.Placeholder = "your-client-key"
	n.Focus()
	keys := make([]string, len(cfg.Auth.ValidAPIKeys))
	copy(keys, cfg.Auth.ValidAPIKeys)
	return authModel{cfg: cfg, back: back, keys: keys, newKey: n, phase: "list"}
}

func (m authModel) Init() tea.Cmd { return textinput.Blink }

func (m authModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.phase == "add" { m.phase = "list"; return m, nil }
			return m.back, nil
		case "enter":
			if m.phase == "add" {
				if v := m.newKey.Value(); v != "" {
					m.keys = append(m.keys, v)
					m.newKey.SetValue("")
				}
				m.phase = "list"
				return m, nil
			}
			m.cfg.Auth.ValidAPIKeys = m.keys
			return m.back, nil
		case "a":
			if m.phase == "list" { m.phase = "add"; m.newKey.SetValue("") }
		case "d", "delete":
			if m.phase == "list" && m.cursor < len(m.keys) {
				m.keys = append(m.keys[:m.cursor], m.keys[m.cursor+1:]...)
				if m.cursor >= len(m.keys) { m.cursor = len(m.keys)-1 }
				if m.cursor < 0 { m.cursor = 0 }
			}
		case "up", "k":
			if m.cursor > 0 { m.cursor-- }
		case "down", "j":
			if m.cursor < len(m.keys)-1 { m.cursor++ }
		}
	}
	if m.phase == "add" {
		var cmd tea.Cmd
		m.newKey, cmd = m.newKey.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m authModel) View() string {
	s := "\n  \033[1mAuth (API Keys)\033[0m\n\n"
	if len(m.keys) == 0 {
		s += "  (no API keys configured \u2014 all clients allowed)\n"
	} else {
		for i, k := range m.keys {
			cursor := "  "
			if m.cursor == i { cursor = "\u25b6 " }
			display := k
			if len(display) > 30 { display = display[:27] + "..." }
			s += cursor + display + "\n"
		}
	}
	if m.phase == "add" {
		s += "\n  New API key: " + m.newKey.View() + "\n"
	}
	s += "\n  a: add | d: remove | enter: save | esc: back"
	return s
}
