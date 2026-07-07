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
	cursor     int // -1 = list, 0+= editing key index
	editFocus  int // 0=name, 1=key, 2=generateBtn
	nameInput  textinput.Model
	keyInput   textinput.Model
}

func newCredentialPage(cfg *SetupConfig) credentialPageModel {
	return credentialPageModel{
		cfg:       cfg,
		cursor:    -1,
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

	if m.cursor >= 0 {
		return m.updateEdit(keyMsg, key)
	}
	return m.updateList(key)
}

func (m credentialPageModel) updateList(key string) (credentialPageModel, tea.Cmd) {
	total := len(m.cfg.ClientKeys) + 1 // keys + [Add] button
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
		if m.cursor >= 0 && m.cursor < len(m.cfg.ClientKeys) {
			m.editFocus = 0
			m.nameInput.SetValue(m.cfg.ClientKeys[m.cursor].Name)
			m.keyInput.SetValue(m.cfg.ClientKeys[m.cursor].Key)
			m.blurAll()
		} else {
			// Add new key (cursor is at [Add] or empty list)
			m.cfg.ClientKeys = append(m.cfg.ClientKeys, ClientKey{})
			m.cursor = len(m.cfg.ClientKeys) - 1
			m.editFocus = 0
			m.nameInput.SetValue("")
			m.keyInput.SetValue("")
			m.blurAll()
		}
	case "d", "delete":
		if m.cursor >= 0 && m.cursor < len(m.cfg.ClientKeys) {
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

func (m credentialPageModel) updateEdit(msg tea.KeyMsg, key string) (credentialPageModel, tea.Cmd) {
	switch key {
	case "esc":
		if m.cursor >= 0 && m.cursor < len(m.cfg.ClientKeys) {
			m.cfg.ClientKeys[m.cursor].Name = m.nameInput.Value()
			m.cfg.ClientKeys[m.cursor].Key = m.keyInput.Value()
		}
		m.cursor = -1
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
			if m.cursor >= 0 && m.cursor < len(m.cfg.ClientKeys) {
				m.cfg.ClientKeys[m.cursor].Key = generateAPIKey()
				m.keyInput.SetValue(m.cfg.ClientKeys[m.cursor].Key)
			}
			m.editFocus = 1
			m.blurAll()
		}
		return m, nil
	}

	m.routeInput(msg)
	return m, nil
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

func (m credentialPageModel) View() string {
	s := "\n" + inputLabelStyle.Render("Client API Keys") + "\n\n"

	if len(m.cfg.ClientKeys) == 0 {
		s += "  (no keys — all clients allowed)\n"
	} else {
		for i, k := range m.cfg.ClientKeys {
			name := k.Name
			if name == "" {
				name = "(unnamed)"
			}
			keyDisplay := k.Key
			if len(keyDisplay) > 20 {
				keyDisplay = keyDisplay[:17] + "..."
			}

			if m.cursor == i && m.editFocus >= 0 {
				// Expanded: show edit form inline
				s += activeTabStyle.Render("▸ "+name) + "\n"
				s += "    " + inputLabelStyle.Render("Name") + m.nameInput.View() + "\n"
				s += "    " + inputLabelStyle.Render("Key") + m.keyInput.View() + "\n"
				s += "    " + renderButtonStatic("Generate", m.editFocus == 2) + "\n"
			} else {
				// Collapsed: just show name + key
				cursor := "  "
				if m.cursor == i {
					cursor = "▸ "
				}
				line := fmt.Sprintf("%s%s  %s", cursor, name, keyDisplay)
				s += normalStyle.Render(line) + "\n"
			}
		}
	}

	// [Add] button
	if m.cursor == -1 {
		s += "\n"
		s += renderButtonStatic("Add", true)
		s += "\n"
	}

	s += "\n"

	if m.cursor >= 0 {
		s += helpText.Render("↑↓: navigate fields | esc: save & back")
	} else {
		s += helpText.Render("↑↓: navigate | enter/→: expand | d: remove")
	}

	return s
}

func renderButtonStatic(label string, highlight bool) string {
	if highlight {
		return activeTabStyle.Render("▸ [" + label + "]")
	}
	return normalStyle.Render("  [" + label + "]")
}
