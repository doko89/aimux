package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ai-router/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type authPageModel struct {
	cfg          *SetupConfig
	editIdx      int // -1 = list mode
	focus        int // edit: 1=name,2=baseurl,3=apikey,4=fetchBtn,5=testBtn,6=saveBtn
	name         textinput.Model
	baseURL      textinput.Model
	apiKey       textinput.Model
	providerList int
	statusMsg    string
	fetching     bool
}

func newAuthPage(cfg *SetupConfig) authPageModel {
	return authPageModel{
		cfg:     cfg,
		editIdx: -1,
		name:    mkInput("", "provider-name"),
		baseURL: mkInput("", "https://api.example.com/v1"),
		apiKey:  mkInput("", "sk-..."),
	}
}

func (m authPageModel) Init() tea.Cmd { return textinput.Blink }

func (m authPageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	new, cmd := m.update(msg, m.cfg)
	return new, cmd
}

func (m authPageModel) update(msg tea.Msg, cfg *SetupConfig) (authPageModel, tea.Cmd) {
	m.cfg = cfg

	switch msg := msg.(type) {
	case fetchModelsDoneMsg:
		m.fetching = false
		if msg.err != "" {
			m.statusMsg = "✗ " + msg.err
		} else {
			m.statusMsg = fmt.Sprintf("✓ Found %d models", len(msg.models))
			if m.editIdx >= 0 && m.editIdx < len(m.cfg.Providers) {
				m.cfg.Providers[m.editIdx].AvailableModels = msg.models
			}
		}
		return m, nil
	case testProviderDoneMsg:
		m.fetching = false
		if msg.err != "" {
			m.statusMsg = "✗ " + msg.err
		} else {
			m.statusMsg = "✓ OK (" + msg.status + ")"
		}
		return m, nil
	case tea.KeyMsg:
		if !m.fetching {
			return m.handleKey(msg)
		}
		return m, nil
	}
	return m, nil
}

func (m authPageModel) handleKey(msg tea.KeyMsg) (authPageModel, tea.Cmd) {
	key := msg.String()
	if m.editIdx >= 0 {
		return m.updateEdit(msg, key)
	}
	return m.updateList(key)
}

// ── LIST MODE ──
// Providers + [Add] button at the bottom

func (m authPageModel) updateList(key string) (authPageModel, tea.Cmd) {
	count := len(m.cfg.Providers)
	addRow := count // index for [Add] button
	total := addRow + 1

	switch key {
	case "up", "k":
		if m.providerList > 0 {
			m.providerList--
		}
	case "down", "j":
		if m.providerList < total-1 {
			m.providerList++
		}
	case "enter":
		if m.providerList < count {
			// Edit existing
			m.editIdx = m.providerList
			m.loadToFields()
			m.focus = 1
		} else if m.providerList == addRow {
			// Add new
			m.editIdx = count
			m.cfg.Providers = append(m.cfg.Providers, ProviderSetup{
				ProviderConfig: config.ProviderConfig{Enabled: true, Weight: 10, Priority: 6, Timeout: 120},
			})
			m.name.SetValue("")
			m.baseURL.SetValue("")
			m.apiKey.SetValue("")
			m.focus = 1
		}
	case "d", "delete":
		if count > 0 && m.providerList < count {
			m.cfg.Providers = append(m.cfg.Providers[:m.providerList], m.cfg.Providers[m.providerList+1:]...)
			if m.providerList >= len(m.cfg.Providers) && m.providerList > 0 {
				m.providerList--
			}
		}
	}
	return m, nil
}

// ── EDIT MODE ──
// Name, Base URL, API Key, [Fetch Model], [Test], Models, [Save]

func (m authPageModel) updateEdit(msg tea.KeyMsg, key string) (authPageModel, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.focus > 1 {
			m.focus--
			m.blurAll()
		}
		return m, nil
	case "down", "j":
		if m.focus < 6 {
			m.focus++
			m.blurAll()
		}
		return m, nil
	case "esc":
		m.commitEdit()
		m.editIdx = -1
		m.focus = 0
		return m, nil
	case "enter":
		switch m.focus {
		case 1, 2, 3:
			// Move to next field
			if m.focus < 3 {
				m.focus++
				m.blurAll()
			} else {
				m.focus = 4
			}
		case 4:
			// Fetch Model
			m.syncInputsToProvider()
			if m.editIdx >= 0 && m.editIdx < len(m.cfg.Providers) {
				p := m.cfg.Providers[m.editIdx]
				if p.BaseURL == "" {
					m.statusMsg = "✗ Base URL required"
					return m, nil
				}
				m.fetching = true
				m.statusMsg = "Fetching..."
				return m, fetchModelsCmd(p.BaseURL, p.APIKey)
			}
		case 5:
			// Test
			m.syncInputsToProvider()
			if m.editIdx >= 0 && m.editIdx < len(m.cfg.Providers) {
				p := m.cfg.Providers[m.editIdx]
				if p.BaseURL == "" {
					m.statusMsg = "✗ Base URL required"
					return m, nil
				}
				m.fetching = true
				m.statusMsg = "Testing..."
				return m, testProviderCmd(p.BaseURL, p.APIKey)
			}
		case 6:
			// Save — commit and return to list
			m.commitEdit()
			m.editIdx = -1
			m.focus = 0
			m.statusMsg = ""
			return m, nil
		}
		return m, nil
	}

	// Non-nav keys → route to active textinput (focus 1-3 only)
	if m.focus >= 1 && m.focus <= 3 {
		m.routeToInput(msg)
	}
	return m, nil
}

func (m *authPageModel) routeToInput(msg tea.KeyMsg) {
	switch m.focus {
	case 1:
		m.name, _ = m.name.Update(msg)
	case 2:
		m.baseURL, _ = m.baseURL.Update(msg)
	case 3:
		m.apiKey, _ = m.apiKey.Update(msg)
	}
}

func (m *authPageModel) blurAll() {
	m.name.Blur()
	m.baseURL.Blur()
	m.apiKey.Blur()
	switch m.focus {
	case 1:
		m.name.Focus()
	case 2:
		m.baseURL.Focus()
	case 3:
		m.apiKey.Focus()
	}
}

func (m *authPageModel) syncInputsToProvider() {
	if m.editIdx < 0 || m.editIdx >= len(m.cfg.Providers) {
		return
	}
	p := &m.cfg.Providers[m.editIdx]
	p.Name = strings.TrimSpace(m.name.Value())
	p.BaseURL = strings.TrimSpace(m.baseURL.Value())
	p.APIKey = strings.TrimSpace(m.apiKey.Value())
	p.Enabled = true
}

func (m *authPageModel) loadToFields() {
	if m.editIdx < 0 || m.editIdx >= len(m.cfg.Providers) {
		return
	}
	p := m.cfg.Providers[m.editIdx]
	m.name.SetValue(p.Name)
	m.baseURL.SetValue(p.BaseURL)
	m.apiKey.SetValue(p.APIKey)
}

func (m *authPageModel) commitEdit() {
	if m.editIdx < 0 || m.editIdx >= len(m.cfg.Providers) {
		return
	}
	p := &m.cfg.Providers[m.editIdx]
	p.Name = strings.TrimSpace(m.name.Value())
	p.BaseURL = strings.TrimSpace(m.baseURL.Value())
	p.APIKey = strings.TrimSpace(m.apiKey.Value())
	p.Enabled = true
	if p.Weight == 0 {
		p.Weight = 10
	}
	if p.Priority == 0 {
		p.Priority = 6
	}
	if p.Timeout == 0 {
		p.Timeout = 120
	}
}

// ── VIEW ──

func (m authPageModel) View() string {
	s := "\n" + inputLabelStyle.Render("Providers") + "\n\n"

	count := len(m.cfg.Providers)

	// Provider list
	if count == 0 {
		s += "  (no providers configured)\n"
	} else {
		for i, p := range m.cfg.Providers {
			cursor := "  "
			if m.editIdx == -1 && m.providerList == i {
				cursor = "▸ "
			} else if m.editIdx == i {
				cursor = "▸ "
			}
			enabled := "off"
			if p.Enabled {
				enabled = "on"
			}
			models := ""
			if len(p.AvailableModels) > 0 {
				models = fmt.Sprintf(" [%d]", len(p.AvailableModels))
			}
			line := fmt.Sprintf("%s%s (%s)%s", cursor, p.Name, enabled, models)
			if m.editIdx == i {
				s += activeTabStyle.Render(line) + "\n"
			} else {
				s += normalStyle.Render(line) + "\n"
			}
		}
	}

	// [Add] button in list mode
	if m.editIdx < 0 {
		s += "\n"
		s += m.renderButton("Add", 0, m.editIdx == -1 && m.providerList == count)
		s += "\n"
	}

	s += "\n"

	// Edit form
	if m.editIdx >= 0 {
		s += helpText.Render("── Edit Provider ──") + "\n\n"
		s += m.renderField("Name", m.name, 1)
		s += m.renderField("Base URL", m.baseURL, 2)
		s += m.renderField("API Key", m.apiKey, 3)
		s += "\n"
		s += m.renderButton("Fetch Model", 4, false) + "  "
		s += m.renderButton("Test", 5, false)
		s += "\n"

		// Models view only
		s += "\n" + inputLabelStyle.Render("Models")
		p := m.cfg.Providers[m.editIdx]
		if len(p.AvailableModels) == 0 {
			s += " (none)\n"
		} else {
			s += fmt.Sprintf(" (%d)\n", len(p.AvailableModels))
			for _, mdl := range p.AvailableModels {
				s += "    " + normalStyle.Render(mdl) + "\n"
			}
		}

		// [Save] button
		s += "\n"
		s += m.renderButton("Save", 6, false)
		s += "\n"

		if m.statusMsg != "" {
			s += "\n" + m.statusMsg + "\n"
		}
	} else {
		s += helpText.Render("enter: edit/add | d: remove")
	}

	return s
}

func (m authPageModel) renderField(label string, input textinput.Model, focus int) string {
	cursor := "  "
	if m.focus == focus {
		cursor = "▸ "
	}
	return cursor + inputLabelStyle.Render(label) + input.View() + "\n"
}

func (m authPageModel) renderButton(label string, focus int, highlight bool) string {
	if highlight {
		return activeTabStyle.Render("▸ [" + label + "]")
	}
	// In edit mode, check focus field
	if m.editIdx >= 0 && m.focus == focus {
		return activeTabStyle.Render("▸ [" + label + "]")
	}
	return normalStyle.Render("  [" + label + "]")
}

// ── COMMANDS ──

type fetchModelsDoneMsg struct {
	models []string
	err    string
}

func fetchModelsCmd(baseURL, apiKey string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		url := strings.TrimRight(baseURL, "/") + "/models"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fetchModelsDoneMsg{err: err.Error()}
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fetchModelsDoneMsg{err: err.Error()}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return fetchModelsDoneMsg{err: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))}
		}

		var listing struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
			return fetchModelsDoneMsg{err: err.Error()}
		}

		var models []string
		for _, m := range listing.Data {
			if m.ID != "" {
				models = append(models, m.ID)
			}
		}
		return fetchModelsDoneMsg{models: models}
	}
}

type testProviderDoneMsg struct {
	status string
	err    string
}

func testProviderCmd(baseURL, apiKey string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		url := strings.TrimRight(baseURL, "/") + "/models"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return testProviderDoneMsg{err: err.Error()}
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return testProviderDoneMsg{err: err.Error()}
		}
		defer resp.Body.Close()

		return testProviderDoneMsg{status: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
}
