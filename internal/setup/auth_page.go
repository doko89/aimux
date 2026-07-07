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

// ─── Auth Page ────────────────────────────────────────────────────

type authPageModel struct {
	cfg          *SetupConfig
	editIdx      int // -1 = list mode
	focus        int // 0=list, 1=name, 2=baseurl, 3=apikey, 4=models
	name         textinput.Model
	baseURL      textinput.Model
	apiKey       textinput.Model
	models       textinput.Model
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
		models:  mkInput("", "model1,model2"),
	}
}

func (m authPageModel) Init() tea.Cmd { return textinput.Blink }

func (m authPageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	new, cmd := m.update(msg, m.cfg)
	return new, cmd
}

func (m authPageModel) update(msg tea.Msg, cfg *SetupConfig) (authPageModel, tea.Cmd) {
	m.cfg = cfg

	// Handle async results
	switch msg := msg.(type) {
	case fetchModelsDoneMsg:
		m.fetching = false
		if msg.err != "" {
			m.statusMsg = "✗ " + msg.err
		} else {
			m.statusMsg = fmt.Sprintf("✓ Found %d models", len(msg.models))
			if m.editIdx >= 0 && m.editIdx < len(m.cfg.Providers) {
				m.cfg.Providers[m.editIdx].AvailableModels = msg.models
				m.models.SetValue(strings.Join(msg.models, ","))
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
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.fetching {
		return m, nil
	}

	key := keyMsg.String()

	// ── EDIT MODE ──
	if m.editIdx >= 0 {
		return m.updateEdit(keyMsg, key)
	}

	// ── LIST MODE ──
	return m.updateList(key)
}

func (m authPageModel) updateList(key string) (authPageModel, tea.Cmd) {
	count := len(m.cfg.Providers)
	switch key {
	case "up", "k":
		if m.providerList > 0 {
			m.providerList--
		}
	case "down", "j":
		if m.providerList < count-1 {
			m.providerList++
		}
	case "enter":
		if m.providerList < count {
			m.editIdx = m.providerList
			m.loadToFields()
			m.focus = 1
			m.name.Focus()
		}
	case "a":
		m.editIdx = count
		m.cfg.Providers = append(m.cfg.Providers, ProviderSetup{
			ProviderConfig: config.ProviderConfig{Enabled: true, Weight: 10, Priority: 6, Timeout: 120},
		})
		m.name.SetValue("")
		m.baseURL.SetValue("")
		m.apiKey.SetValue("")
		m.models.SetValue("")
		m.focus = 1
		m.name.Focus()
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

func (m authPageModel) updateEdit(msg tea.KeyMsg, key string) (authPageModel, tea.Cmd) {
	// Navigation keys — handle FIRST, never send to textinput
	switch key {
	case "up", "k":
		if m.focus > 1 {
			m.focus--
			m.focusInput()
		}
		return m, nil
	case "down", "j":
		if m.focus < 4 {
			m.focus++
			m.focusInput()
		}
		return m, nil
	case "esc":
		m.commitEdit()
		m.editIdx = -1
		m.focus = 0
		return m, nil
	case "tab":
		if m.focus < 4 {
			m.focus++
		} else {
			m.commitEdit()
			m.editIdx = -1
			m.focus = 0
		}
		m.focusInput()
		return m, nil
	case "shift+tab":
		if m.focus > 1 {
			m.focus--
		}
		m.focusInput()
		return m, nil
	case "enter":
		// For text fields, enter moves to next; on last field, save
		if m.focus < 4 {
			m.focus++
			m.focusInput()
		} else {
			m.commitEdit()
			m.editIdx = -1
			m.focus = 0
		}
		return m, nil
	case "e":
		if m.editIdx >= 0 && m.editIdx < len(m.cfg.Providers) {
			m.fetching = true
			m.statusMsg = "Fetching..."
			p := m.cfg.Providers[m.editIdx]
			return m, fetchModelsCmd(p.BaseURL, p.APIKey)
		}
	case "t":
		if m.editIdx >= 0 && m.editIdx < len(m.cfg.Providers) {
			m.fetching = true
			m.statusMsg = "Testing..."
			p := m.cfg.Providers[m.editIdx]
			return m, testProviderCmd(p.BaseURL, p.APIKey)
		}
	}

	// All other keys (typing, left/right, backspace) → textinput
	if m.focus >= 1 && m.focus <= 4 {
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
	case 4:
		m.models, _ = m.models.Update(msg)
	}
}

func (m *authPageModel) focusInput() {
	m.name.Blur()
	m.baseURL.Blur()
	m.apiKey.Blur()
	m.models.Blur()
	switch m.focus {
	case 1:
		m.name.Focus()
	case 2:
		m.baseURL.Focus()
	case 3:
		m.apiKey.Focus()
	case 4:
		m.models.Focus()
	}
}

func (m *authPageModel) loadToFields() {
	if m.editIdx < 0 || m.editIdx >= len(m.cfg.Providers) {
		return
	}
	p := m.cfg.Providers[m.editIdx]
	m.name.SetValue(p.Name)
	m.baseURL.SetValue(p.BaseURL)
	m.apiKey.SetValue(p.APIKey)
	m.models.SetValue(strings.Join(p.AvailableModels, ","))
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
	modelsRaw := strings.TrimSpace(m.models.Value())
	if modelsRaw != "" {
		var ms []string
		for _, s := range strings.Split(modelsRaw, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				ms = append(ms, s)
			}
		}
		p.AvailableModels = ms
	}
}

func (m authPageModel) View() string {
	s := "\n" + inputLabelStyle.Render("Providers") + "\n\n"

	if len(m.cfg.Providers) == 0 {
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

	s += "\n"

	if m.editIdx >= 0 {
		s += helpText.Render("── Edit Provider ──") + "\n\n"
		s += m.renderField("Name", m.name, 1)
		s += m.renderField("Base URL", m.baseURL, 2)
		s += m.renderField("API Key", m.apiKey, 3)
		s += m.renderField("Models", m.models, 4)
		s += "\n"
		s += "  " + btnStyle.Render(" e:Fetch ") + "   "
		s += btnStyle.Render(" t:Test ") + "   "
		s += btnStyle.Render(" esc:Done ") + "\n"
	} else {
		s += helpText.Render("enter: edit | a: add | d: remove") + "\n"
	}

	if m.statusMsg != "" {
		s += "\n" + m.statusMsg + "\n"
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

// ─── Commands ─────────────────────────────────────────────────────

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
