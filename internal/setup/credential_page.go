package setup

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type credentialPageModel struct {
	cfg        *SetupConfig
	cursor     int  // -1 = list mode
	expanded   int  // -1 = none expanded, else index of expanded key
	editFocus  int  // 0=name, 1=key, 2=generateBtn
	nameInput  textinput.Model
	keyInput   textinput.Model
}

func newCredentialPage(cfg *SetupConfig) credentialPageModel {
	return credentialPageModel{
		cfg:       cfg,
		cursor:    -1,
		expanded:  -1,
		nameInput: mkInput("", "key name"),
		keyInput:  mkInput("", "api-key-xxx"),
	}
}

func (m credentialPageModel) Init() tea.Cmd { return textinput.Blink }

func (m credentialPageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	new, cmd := m.update(msg, m.cfg)
	return new, cmd
}

func (m credentialPageModel) update(msg tea.Msg, cfg *SetupConfig) (credentialPageModel, tea.Cmd) {
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

// ── LIST MODE (not expanded) ──

func (m credentialPageModel) updateList(key string) (credentialPageModel, tea.Cmd) {
	count := len(m.cfg.ClientKeys)
	total := count + 1 // keys + [Add]
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
			// Expand for editing
			m.expanded = m.cursor
			m.editFocus = 0
			m.nameInput.SetValue(m.cfg.ClientKeys[m.cursor].Name)
			m.keyInput.SetValue(m.cfg.ClientKeys[m.cursor].Key)
			m.blurAll()
		} else if m.cursor >= count || m.cursor == -1 {
			// Add new
			m.cfg.ClientKeys = append(m.cfg.ClientKeys, ClientKey{})
			m.cursor = len(m.cfg.ClientKeys) - 1
			m.expanded = m.cursor
			m.editFocus = 0
			m.nameInput.SetValue("")
			m.keyInput.SetValue("")
			m.blurAll()
		}
	case "d", "delete":
		if m.cursor >= 0 && m.cursor < count {
			m.cfg.ClientKeys = append(m.cfg.ClientKeys[:m.cursor], m.cfg.ClientKeys[m.cursor+1:]...)
			if m.cursor >= len(m.cfg.ClientKeys) {
				m.cursor = len(m.cfg.ClientKeys) - 1
			}
			if m.cursor < -1 {
				m.cursor = -1
			}
		}
	}
	return m, nil
}

// ── EXPANDED MODE (editing) ──

func (m credentialPageModel) updateEdit(msg tea.KeyMsg, key string) (credentialPageModel, tea.Cmd) {
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
		if m.editFocus < 2 {
			m.editFocus++
			m.blurAll()
		}
		return m, nil
	case "enter":
		switch m.editFocus {
		case 0:
			m.editFocus = 1
			m.blurAll()
		case 1:
			m.editFocus = 2
			m.blurAll()
		case 2:
			if m.expanded >= 0 && m.expanded < len(m.cfg.ClientKeys) {
				m.cfg.ClientKeys[m.expanded].Key = generateAPIKey()
				m.keyInput.SetValue(m.cfg.ClientKeys[m.expanded].Key)
			}
			m.editFocus = 1
			m.blurAll()
		}
		return m, nil
	}

	// Typing
	m.routeInput(msg)
	return m, nil
}

func (m *credentialPageModel) saveExpand() {
	if m.expanded >= 0 && m.expanded < len(m.cfg.ClientKeys) {
		m.cfg.ClientKeys[m.expanded].Name = m.nameInput.Value()
		m.cfg.ClientKeys[m.expanded].Key = m.keyInput.Value()
	}
	m.expanded = -1
	m.cursor = -1
}

func (m *credentialPageModel) routeInput(msg tea.KeyMsg) {
	switch m.editFocus {
	case 0:
		m.nameInput, _ = m.nameInput.Update(msg)
	case 1:
		m.keyInput, _ = m.keyInput.Update(msg)
	}
}

func (m *credentialPageModel) blurAll() {
	m.nameInput.Blur()
	m.keyInput.Blur()
	switch m.editFocus {
	case 0:
		m.nameInput.Focus()
	case 1:
		m.keyInput.Focus()
	}
}

func generateAPIKey() string {
	b := make([]byte, 24)
	rand.Read(b)
	return "ak-" + hex.EncodeToString(b)
}

func renderButtonStatic(label string, highlight bool) string {
	if highlight {
		return activeTabStyle.Render("▸ [" + label + "]")
	}
	return normalStyle.Render("  [" + label + "]")
}


func (m credentialPageModel) View() string {
	s := "\n" + inputLabelStyle.Render("Client API Keys") + "\n\n"

	count := len(m.cfg.ClientKeys)
	keyDisplay := func(k ClientKey) string {
		if len(k.Key) > 22 {
			return k.Key[:19] + "..."
		}
		return k.Key
	}

	if count == 0 {
		s += "  (no keys — all clients allowed)\n"
	} else {
		for i, k := range m.cfg.ClientKeys {
			name := k.Name
			if name == "" {
				name = "(unnamed)"
			}

			if m.expanded == i {
				// Expanded: show edit form inline
				s += activeTabStyle.Render("▸ "+name) + "\n"
				s += "    " + inputLabelStyle.Render("Name") + m.nameInput.View() + "\n"
				s += "    " + inputLabelStyle.Render("Key") + m.keyInput.View() + "\n"
				s += "    " + renderButtonStatic("Generate", m.editFocus == 2) + "\n"
			} else {
				// Collapsed: cursor + name + full key
				cursor := "  "
				if m.expanded == -1 && m.cursor == i {
					cursor = "▸ "
				}
				s += fmt.Sprintf("%s%s  %s\n", cursor, name, keyDisplay(k))
			}
		}
	}

	// [Add] button (only in list mode)
	s += "\n"
	showAdd := m.expanded == -1 && (m.cursor >= count || m.cursor == -1)
	if showAdd {
		s += renderButtonStatic("Add", m.cursor >= count || count == 0)
	} else {
		s += normalStyle.Render("  [Add]")
	}
	s += "\n"

	s += "\n"

	if m.expanded >= 0 {
		s += helpText.Render("↑↓: navigate | esc/←: save & back")
	} else {
		s += helpText.Render("↑↓: navigate | enter/→: edit | d: remove")
	}

	return s
}
