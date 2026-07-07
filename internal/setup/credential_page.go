package setup

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type clientKey struct {
	Name string
	Key  string
}

type credentialPageModel struct {
	cfg        *SetupConfig
	keys       []clientKey
	cursor     int // -1 = list, 0+= editing key index
	editFocus  int // 0=name, 1=key, 2=generateBtn
	nameInput  textinput.Model
	keyInput   textinput.Model
}

func newCredentialPage(cfg *SetupConfig) credentialPageModel {
	// Load existing keys from config
	var keys []clientKey
	for _, k := range cfg.ClientKeys {
		keys = append(keys, clientKey{Key: k})
	}

	return credentialPageModel{
		cfg:       cfg,
		keys:      keys,
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

	// List mode
	if m.cursor == -1 {
		return m.updateList(key)
	}
	return m.updateEdit(keyMsg, key)
}

func (m credentialPageModel) updateList(key string) (credentialPageModel, tea.Cmd) {
	total := len(m.keys) + 1 // keys + [Add] button
	switch key {
	case "up", "k":
		if m.cursor > -1 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < total-2 { // don't go past [Add]
			m.cursor++
		}
	case "enter":
		if m.cursor < len(m.keys) {
			// Edit existing key
			m.editFocus = 0
			m.nameInput.SetValue(m.keys[m.cursor].Name)
			m.keyInput.SetValue(m.keys[m.cursor].Key)
		} else {
			// Add new key
			m.keys = append(m.keys, clientKey{})
			m.cursor = len(m.keys) - 1
			m.editFocus = 0
			m.nameInput.SetValue("")
			m.keyInput.SetValue("")
		}
	case "d", "delete":
		if m.cursor >= 0 && m.cursor < len(m.keys) {
			m.keys = append(m.keys[:m.cursor], m.keys[m.cursor+1:]...)
			if m.cursor >= len(m.keys) {
				m.cursor = len(m.keys) - 1
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
		// Save and return to list
		if m.cursor >= 0 && m.cursor < len(m.keys) {
			m.keys[m.cursor].Name = m.nameInput.Value()
			m.keys[m.cursor].Key = m.keyInput.Value()
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
			// Generate random key
			if m.cursor >= 0 && m.cursor < len(m.keys) {
				m.keys[m.cursor].Key = generateAPIKey()
				m.keyInput.SetValue(m.keys[m.cursor].Key)
			}
			m.editFocus = 1
			m.blurAll()
		}
		return m, nil
	}

	// Typing → route to active input
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

// syncKeys writes keys back to config
func (m *credentialPageModel) syncKeys() {
	var keys []string
	for _, k := range m.keys {
		if k.Key != "" {
			keys = append(keys, k.Key)
		}
	}
	m.cfg.ClientKeys = keys
}

// ── VIEW ──

func (m credentialPageModel) View() string {
	s := "\n" + inputLabelStyle.Render("Client API Keys") + "\n\n"

	if len(m.keys) == 0 {
		s += "  (no keys — all clients allowed)\n"
	} else {
		for i, k := range m.keys {
			cursor := "  "
			if m.cursor == i {
				cursor = "▸ "
			}
			name := k.Name
			if name == "" {
				name = "(unnamed)"
			}
			keyDisplay := k.Key
			if len(keyDisplay) > 20 {
				keyDisplay = keyDisplay[:17] + "..."
			}
			line := fmt.Sprintf("%s%s  %s", cursor, name, keyDisplay)
			if m.cursor == i && m.editFocus == 0 {
				s += activeTabStyle.Render(line) + "\n"
			} else {
				s += normalStyle.Render(line) + "\n"
			}
		}
	}

	// [Add] button in list mode
	if m.cursor == -1 {
		s += "\n"
		s += renderButtonStatic("Add", true)
		s += "\n"
	}

	s += "\n"

	// Edit form
	if m.cursor >= 0 {
		s += helpText.Render("── Edit Key ──") + "\n\n"
		cursor := "  "
		if m.editFocus == 0 {
			cursor = "▸ "
		}
		s += cursor + inputLabelStyle.Render("Name") + m.nameInput.View() + "\n"
		cursor = "  "
		if m.editFocus == 1 {
			cursor = "▸ "
		}
		s += cursor + inputLabelStyle.Render("Key") + m.keyInput.View() + "\n"
		s += "\n"
		s += renderButtonStatic("Generate", m.editFocus == 2)
		s += "\n"
		s += helpText.Render("esc: save & back")
	} else {
		s += helpText.Render("enter: edit/add | d: remove")
	}

	return s
}

func renderButtonStatic(label string, highlight bool) string {
	if highlight {
		return activeTabStyle.Render("▸ [" + label + "]")
	}
	return normalStyle.Render("  [" + label + "]")
}
