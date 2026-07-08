package setup

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type mcpPageModel struct {
	cfg          *SetupConfig
	cursor       int  // -1 = list mode
	expanded     int  // -1 = none expanded, else index of expanded server
	editFocus    int  // 0=name, 1=url, 2=token, 3=timeout, 4=prefix, 5=enabled toggle, 6=save
	nameInput    textinput.Model
	urlInput     textinput.Model
	timeoutInput textinput.Model
	prefixInput  textinput.Model
	tokenInput   textinput.Model
	enabled      bool
}

func newMCPPage(cfg *SetupConfig) mcpPageModel {
	return mcpPageModel{
		cfg:          cfg,
		cursor:       -1,
		expanded:     -1,
		nameInput:    mkInput("", "server-name"),
		urlInput:     mkInput("", "http://localhost:3001/mcp"),
		timeoutInput: mkInput("10", "timeout (s)"),
		prefixInput:  mkInput("", "tool_prefix:"),
		tokenInput:   mkInput("", "Bearer token (${ENV_VAR})"),
	}
}

func (m mcpPageModel) Init() tea.Cmd { return textinput.Blink }

func (m mcpPageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	new, cmd := m.update(msg, m.cfg)
	return new, cmd
}

func (m mcpPageModel) update(msg tea.Msg, cfg *SetupConfig) (mcpPageModel, tea.Cmd) {
	m.cfg = cfg

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	key := keyMsg.String()

	if m.expanded >= 0 {
		return m.updateEdit(keyMsg, key)
	}
	return m.updateList(key)
}

// ── LIST MODE ──

func (m mcpPageModel) updateList(key string) (mcpPageModel, tea.Cmd) {
	count := len(m.cfg.MCPServers)
	total := count + 1 // servers + [Add]
	switch key {
	case "up", "k":
		if m.cursor > -1 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < total-1 {
			m.cursor++
		}
	case "enter", "right":
		if m.cursor >= 0 && m.cursor < count {
			m.expanded = m.cursor
			m.editFocus = 0
			ms := m.cfg.MCPServers[m.cursor]
			m.nameInput.SetValue(ms.Name)
			m.urlInput.SetValue(ms.URL)
			m.timeoutInput.SetValue(strconv.Itoa(ms.Timeout))
			m.prefixInput.SetValue(ms.ToolPrefix)
			m.tokenInput.SetValue(ms.BearerToken)
			m.enabled = ms.Enabled
			m.blurAll()
		} else if m.cursor >= count || m.cursor == -1 {
			m.cfg.MCPServers = append(m.cfg.MCPServers, MCPServerSetup{
				Timeout: 10,
				Enabled: true,
			})
			m.cursor = len(m.cfg.MCPServers) - 1
			m.expanded = m.cursor
			m.editFocus = 0
			m.nameInput.SetValue("")
			m.urlInput.SetValue("")
			m.timeoutInput.SetValue("10")
			m.prefixInput.SetValue("")
			m.tokenInput.SetValue("")
			m.enabled = true
			m.blurAll()
		}
	case "d", "delete":
		if m.cursor >= 0 && m.cursor < count {
			m.cfg.MCPServers = append(m.cfg.MCPServers[:m.cursor], m.cfg.MCPServers[m.cursor+1:]...)
			if m.cursor >= len(m.cfg.MCPServers) {
				m.cursor = len(m.cfg.MCPServers) - 1
			}
			if m.cursor < -1 {
				m.cursor = -1
			}
		}
	}
	return m, nil
}

// ── EDIT MODE ──

func (m mcpPageModel) updateEdit(msg tea.KeyMsg, key string) (mcpPageModel, tea.Cmd) {
	switch key {
	case "esc", "left":
		m.saveExpand()
		return m, nil
	case "up", "k":
		if m.editFocus > 0 {
			m.editFocus--
			m.blurAll()
		}
		return m, nil
	case "down", "j":
		if m.editFocus < 6 {
			m.editFocus++
			m.blurAll()
		}
		return m, nil
	case "enter":
		switch m.editFocus {
		case 0:
			m.editFocus = 1
		case 1:
			m.editFocus = 2
		case 2:
			m.editFocus = 3
		case 3:
			m.editFocus = 4
		case 4:
			m.editFocus = 5
		case 5:
			m.enabled = !m.enabled
			return m, nil
		case 6:
			m.saveExpand()
			return m, nil
		}
		m.blurAll()
		return m, nil
	case " ":
		if m.editFocus == 5 {
			m.enabled = !m.enabled
			return m, nil
		}
	}

	// Typing
	m.routeInput(msg)
	return m, nil
}

func (m *mcpPageModel) saveExpand() {
	if m.expanded >= 0 && m.expanded < len(m.cfg.MCPServers) {
		timeout := 10
		if v, err := strconv.Atoi(m.timeoutInput.Value()); err == nil && v > 0 {
			timeout = v
		}
		m.cfg.MCPServers[m.expanded] = MCPServerSetup{
			Name:        m.nameInput.Value(),
			URL:         m.urlInput.Value(),
			Timeout:     timeout,
			ToolPrefix:  m.prefixInput.Value(),
			BearerToken: m.tokenInput.Value(),
			Enabled:     m.enabled,
		}
	}
	m.expanded = -1
	m.cursor = -1
}

func (m *mcpPageModel) routeInput(msg tea.KeyMsg) {
	switch m.editFocus {
	case 0:
		m.nameInput, _ = m.nameInput.Update(msg)
	case 1:
		m.urlInput, _ = m.urlInput.Update(msg)
	case 2:
		m.tokenInput, _ = m.tokenInput.Update(msg)
	case 3:
		m.timeoutInput, _ = m.timeoutInput.Update(msg)
	case 4:
		m.prefixInput, _ = m.prefixInput.Update(msg)
	}
}

func (m *mcpPageModel) blurAll() {
	m.nameInput.Blur()
	m.urlInput.Blur()
	m.timeoutInput.Blur()
	m.prefixInput.Blur()
	m.tokenInput.Blur()
	switch m.editFocus {
	case 0:
		m.nameInput.Focus()
	case 1:
		m.urlInput.Focus()
	case 2:
		m.tokenInput.Focus()
	case 3:
		m.timeoutInput.Focus()
	case 4:
		m.prefixInput.Focus()
	}
}

// ── VIEW ──

func (m mcpPageModel) View() string {
	s := "\n" + inputLabelStyle.Render("MCP Servers") + "\n\n"

	count := len(m.cfg.MCPServers)
	if count == 0 {
		s += "  (no MCP servers configured)\n"
	} else {
		for i, ms := range m.cfg.MCPServers {
			if m.expanded == i {
				s += activeTabStyle.Render("▸ "+ms.Name) + "\n"
				s += "    " + inputLabelStyle.Render("Name") + clipView(m.nameInput.View(), 50) + "\n"
				s += "    " + inputLabelStyle.Render("URL") + clipView(m.urlInput.View(), 50) + "\n"
				s += "    " + inputLabelStyle.Render("Token") + clipView(m.tokenInput.View(), 50) + "\n"
				s += "    " + inputLabelStyle.Render("Timeout(s)") + clipView(m.timeoutInput.View(), 20) + "\n"
				s += "    " + inputLabelStyle.Render("Prefix") + clipView(m.prefixInput.View(), 30) + "\n"
				enStr := "off"
				if m.enabled {
					enStr = "on"
				}
				s += "    " + inputLabelStyle.Render("Enabled") + enStr + " (space)\n"
			} else {
				cursor := "  "
				if m.expanded == -1 && m.cursor == i {
					cursor = "▸ "
				}
				enMarker := "[off]"
				if ms.Enabled {
					enMarker = "[on]"
				}
				prefix := ""
				if ms.ToolPrefix != "" {
					prefix = " prefix=" + ms.ToolPrefix
				}
				token := ""
				if ms.BearerToken != "" {
					token = " token=***"
				}
				s += fmt.Sprintf("%s%s  %s  %s%s%s\n", cursor, ms.Name, ms.URL, enMarker, prefix, token)
			}
		}
	}

	s += "\n"
	if m.expanded == -1 {
		if m.cursor >= count || m.cursor == -1 {
			s += renderButtonStatic("Add", true)
		} else {
			s += renderButtonStatic("Add", false)
		}
	} else {
		s += normalStyle.Render("  [Add]")
	}
	s += "\n"

	s += "\n"
	if m.expanded >= 0 {
		s += helpText.Render("↑↓: navigate | space: toggle | enter: next/save | esc: save & back")
	} else {
		s += helpText.Render("↑↓: navigate | enter/→: edit | d: remove")
	}

	return s
}
