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

// ─── Auth Page: manage providers ──────────────────────────────────

type authPageModel struct {
	cfg           *SetupConfig
	focus         int // 0=provider list, 1=name, 2=baseurl, 3=apikey, 4=buttons
	editIdx       int // -1 = new provider
	name          textinput.Model
	baseURL       textinput.Model
	apiKey        textinput.Model
	models        textinput.Model
	providerList  int
	statusMsg     string
	fetching      bool
}

func newAuthPage(cfg *SetupConfig) authPageModel {
	n := textinput.New()
	n.Placeholder = "provider-name"
	bu := textinput.New()
	bu.Placeholder = "https://api.example.com/v1"
	ak := textinput.New()
	ak.Placeholder = "sk-..."
	ak.EchoMode = textinput.EchoPassword
	md := textinput.New()
	md.Placeholder = "model1,model2,model3"

	return authPageModel{
		cfg:     cfg,
		focus:   0,
		editIdx: -1,
		name:    n,
		baseURL: bu,
		apiKey:  ak,
		models:  md,
	}
}

func (m authPageModel) Init() tea.Cmd { return textinput.Blink }

func (m authPageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			m.statusMsg = "✓ Connection OK (" + msg.status + ")"
		}
		return m, nil
	case tea.KeyMsg:
		if m.fetching {
			return m, nil
		}
		return m.handleKeyMsg(msg)
	}
	return m, nil
}

func (m authPageModel) handleKeyMsg(msg tea.KeyMsg) (authPageModel, tea.Cmd) {
	key := msg.String()
	navKeys := map[string]bool{
		"tab": true, "shift+tab": true, "up": true, "k": true,
		"down": true, "j": true, "enter": true, "escape": true, "esc": true,
		"a": true, "A": true, "d": true, "delete": true, "e": true, "t": true,
	}

	// If editing and focus is on text input, route non-nav keys to input
	if m.focus >= 1 && m.focus <= 4 && !navKeys[key] {
		return m.routeToInput(msg), nil
	}

	// Navigation keys
	switch key {
	case "up", "k":
		if m.editIdx == -1 && m.providerList > 0 {
			m.providerList--
		}
	case "down", "j":
		if m.editIdx == -1 {
			max := len(m.cfg.Providers)
			if m.providerList < max-1 {
				m.providerList++
			}
		}
	case "tab":
		if m.editIdx >= 0 {
			if m.focus < 4 {
				m.focus++
			} else {
				m.focus = 0
				m.commitEdit()
				m.editIdx = -1
			}
		}
	case "shift+tab":
		if m.editIdx >= 0 && m.focus > 0 {
			m.focus--
		}
	case "enter":
		if m.editIdx == -1 && m.providerList < len(m.cfg.Providers) {
			m.editIdx = m.providerList
			m.loadToFields()
			m.focus = 1
		}
	case "a", "A":
		if m.editIdx == -1 {
			m.newProvider()
		}
	case "d", "delete":
		if m.editIdx == -1 && m.providerList < len(m.cfg.Providers) {
			m.cfg.Providers = append(m.cfg.Providers[:m.providerList], m.cfg.Providers[m.providerList+1:]...)
			if m.providerList >= len(m.cfg.Providers) && m.providerList > 0 {
				m.providerList--
			}
		}
	case "e":
		if m.editIdx >= 0 && m.editIdx < len(m.cfg.Providers) {
			m.fetching = true
			m.statusMsg = "Fetching models..."
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
	case "escape", "esc":
		if m.editIdx >= 0 {
			m.commitEdit()
			m.editIdx = -1
			m.focus = 0
		}
	}
	return m, nil
}

func (m authPageModel) routeToInput(msg tea.KeyMsg) authPageModel {
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
	return m
}

func (m *authPageModel) newProvider() {
	m.editIdx = len(m.cfg.Providers)
	m.name.SetValue("")
	m.baseURL.SetValue("")
	m.apiKey.SetValue("")
	m.models.SetValue("")
	m.focus = 1
	m.cfg.Providers = append(m.cfg.Providers, ProviderSetup{
		ProviderConfig: config.ProviderConfig{
			Enabled: true, Weight: 10, Priority: 6, Timeout: 120,
		},
	})
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
	s := "\n"

	s += inputLabelStyle.Render("Providers") + "\n"
	if len(m.cfg.Providers) == 0 {
		s += "  (no providers configured)\n"
	} else {
		for i, p := range m.cfg.Providers {
			cursor := "  "
			indicator := "○"
			if m.editIdx == i {
				cursor = "▸ "
				indicator = "●"
			}
			enabled := "off"
			if p.Enabled {
				enabled = "on"
			}
			models := ""
			if len(p.AvailableModels) > 0 {
				models = fmt.Sprintf(" [%d models]", len(p.AvailableModels))
			}
			line := fmt.Sprintf("%s %s %s (%s)%s", cursor, indicator, p.Name, enabled, models)
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
		fields := []struct {
			label string
			input textinput.Model
		}{
			{"Name", m.name},
			{"Base URL", m.baseURL},
			{"API Key", m.apiKey},
			{"Models", m.models},
		}
		for idx, f := range fields {
			cursor := "  "
			if m.focus == idx+1 {
				cursor = "▸ "
			}
			s += cursor + inputLabelStyle.Render(f.label) + f.input.View() + "\n"
		}
		s += "\n"
		s += "  " + btnStyle.Render(" e:Fetch ") + "   "
		s += btnStyle.Render(" t:Test ") + "   "
		s += btnStyle.Render(" esc:Save ") + "\n"
	} else {
		s += helpText.Render("a: add | enter: edit | d: remove | s: save all") + "\n"
	}

	if m.statusMsg != "" {
		s += "\n" + m.statusMsg + "\n"
	}

	return s
}

// ─── Fetch models command ─────────────────────────────────────────

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
			req.Header.Set("Authorization", "Bearer " + apiKey)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fetchModelsDoneMsg{err: err.Error()}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return fetchModelsDoneMsg{
				err: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
			}
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

// ─── Test provider command ────────────────────────────────────────

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
			req.Header.Set("Authorization", "Bearer " + apiKey)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return testProviderDoneMsg{err: err.Error()}
		}
		defer resp.Body.Close()

		return testProviderDoneMsg{status: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
}
