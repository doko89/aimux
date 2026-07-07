package setup

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"ai-router/internal/config"
)

// --- Provider List ---

type providerListModel struct {
	cfg    *SetupConfig
	back   *mainMenuModel
	cursor int
	action string // "", "add", "edit", "remove"
}

func newProviderListModel(cfg *SetupConfig, back *mainMenuModel) providerListModel {
	return providerListModel{cfg: cfg, back: back}
}

func (m providerListModel) Init() tea.Cmd { return nil }

func (m providerListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if isBackMsg(msg) {
		return m.back, nil
	}
	count := len(m.cfg.Providers)
	total := count + 2 // Add + separator + Back

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m.back, nil
		case "up", "k":
			if m.cursor > 0 { m.cursor-- }
		case "down", "j":
			if m.cursor < total-1 { m.cursor++ }
		case "enter":
			if m.cursor == count {
				// Add new provider — show type selector
				return newProviderAddModel(m.cfg, &m), nil
			} else if m.cursor == count+1 {
				return m.back, nil
			} else {
				// Edit existing
				return newProviderEditModel(m.cfg, &m, m.cursor), nil
			}
		case "delete", "x":
			if m.cursor < count {
				// Remove provider
				m.cfg.Providers = append(m.cfg.Providers[:m.cursor], m.cfg.Providers[m.cursor+1:]...)
				if m.cursor >= len(m.cfg.Providers) { m.cursor = len(m.cfg.Providers) }
			}
		}
	}
	return m, nil
}

func (m providerListModel) View() string {
	s := "\n  \033[1mProviders\033[0m\n\n"
	for i, p := range m.cfg.Providers {
		cursor := "  "
		if m.cursor == i { cursor = "\u25b6 " }
		enabled := "\033[31moff\033[0m"
		if p.Enabled { enabled = "\033[32mon\033[0m" }
		s += fmt.Sprintf("%s %s [%s] %s (w:%d p:%d)\n", cursor, p.Name, enabled, p.Model, p.Weight, p.Priority)
	}
	s += "  \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\n"
	addCursor := "  "
	if m.cursor == len(m.cfg.Providers) { addCursor = "\u25b6 " }
	s += addCursor + "Add new provider\n"
	backCursor := "  "
	if m.cursor == len(m.cfg.Providers)+1 { backCursor = "\u25b6 " }
	s += backCursor + "Back\n"
	s += "\n  enter: edit | delete/x: remove | esc: back"
	return s
}

// --- Add Provider (type selector) ---

type providerAddModel struct {
	cfg    *SetupConfig
	back   tea.Model
	cursor int
}

func newProviderAddModel(cfg *SetupConfig, back tea.Model) providerAddModel {
	return providerAddModel{cfg: cfg, back: back}
}

func (m providerAddModel) Init() tea.Cmd { return nil }

func (m providerAddModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	types := AvailableProviderTypes()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m.back, nil
		case "up", "k":
			if m.cursor > 0 { m.cursor-- }
		case "down", "j":
			if m.cursor < len(types)-1 { m.cursor++ }
		case "enter":
			selected := types[m.cursor]
			p := ProviderDefaults(selected)
			return newProviderEditModelFull(m.cfg, m.back, p), nil
		}
	}
	return m, nil
}

func (m providerAddModel) View() string {
	s := "\n  \033[1mAdd Provider\u2014Select Type\033[0m\n\n"
	for i, t := range AvailableProviderTypes() {
		cursor := "  "
		if m.cursor == i { cursor = "\u25b6 " }
		s += cursor + t + "\n"
	}
	s += "\n  enter: select | esc: back"
	return s
}

// --- Edit Provider ---

type providerEditModel struct {
	cfg      *SetupConfig
	back     tea.Model
	idx      int
	name     textinput.Model
	baseURL  textinput.Model
	apiKey   textinput.Model
	model    textinput.Model
	weight   textinput.Model
	priority textinput.Model
	enabled  bool
	fields   []string
	cursor   int
}

func newProviderEditModel(cfg *SetupConfig, back tea.Model, idx int) providerEditModel {
	p := cfg.Providers[idx]
	return newProviderEditModelFull(cfg, back, p)
}

func newProviderEditModelFull(cfg *SetupConfig, back tea.Model, p config.ProviderConfig) providerEditModel {
	name := textinput.New()
	name.SetValue(p.Name)
	name.Focus()

	base := textinput.New()
	base.SetValue(p.BaseURL)
	base.Placeholder = "https://api.example.com/v1"

	key := textinput.New()
	key.SetValue(p.APIKey)
	key.Placeholder = "sk-..."

	mdl := textinput.New()
	mdl.SetValue(p.Model)
	mdl.Placeholder = "model-name"

	w := textinput.New()
	w.SetValue(strconv.Itoa(p.Weight))

	pri := textinput.New()
	pri.SetValue(strconv.Itoa(p.Priority))

	return providerEditModel{
		cfg:     cfg,
		back:    back,
		name:    name,
		baseURL: base,
		apiKey:  key,
		model:   mdl,
		weight:  w,
		priority: pri,
		enabled: p.Enabled,
		fields:  []string{"Name", "Base URL", "API Key", "Model", "Weight", "Priority", "Enabled"},
	}
}

func (m providerEditModel) Init() tea.Cmd { return textinput.Blink }

func (m providerEditModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m.back, nil
		case "tab", "down":
			if m.cursor < len(m.fields)-1 { m.cursor++ }
		case "up":
			if m.cursor > 0 { m.cursor-- }
		case " ":
			if m.cursor == 6 { m.enabled = !m.enabled }
		case "enter":
			// Build provider
			weight := 10
			if v, err := strconv.Atoi(m.weight.Value()); err == nil { weight = v }
			pri := 6
			if v, err := strconv.Atoi(m.priority.Value()); err == nil { pri = v }

			p := config.ProviderConfig{
				Name:     strings.TrimSpace(m.name.Value()),
				Enabled:  m.enabled,
				BaseURL:  strings.TrimSpace(m.baseURL.Value()),
				APIKey:   strings.TrimSpace(m.apiKey.Value()),
				Model:    strings.TrimSpace(m.model.Value()),
				Weight:   weight,
				Priority: pri,
				Timeout:  120,
			}

			// Check if editing existing or new
			found := false
			for i, existing := range m.cfg.Providers {
				if existing.Name == p.Name {
					m.cfg.Providers[i] = p
					found = true
					break
				}
			}
			if !found {
				m.cfg.Providers = append(m.cfg.Providers, p)
			}

			// If this was opened from provider list model, return to list
			if plm, ok := m.back.(*providerListModel); ok {
				return plm, nil
			}
			return m.back, nil
		}
	}

	var cmd tea.Cmd
	inputs := []textinput.Model{m.name, m.baseURL, m.apiKey, m.model, m.weight, m.priority}
	if m.cursor < len(inputs) {
		inputs[m.cursor], cmd = inputs[m.cursor].Update(msg)
	}
	return m, cmd
}

func (m providerEditModel) View() string {
	s := "\n  \033[1mEdit Provider\033[0m\n\n"
	s += "  Name:     " + m.name.View() + "\n"
	s += "  Base URL: " + m.baseURL.View() + "\n"
	s += "  API Key:  " + m.apiKey.View() + "\n"
	s += "  Model:    " + m.model.View() + "\n"
	s += "  Weight:   " + m.weight.View() + "\n"
	s += "  Priority: " + m.priority.View() + "\n"
	enabledStr := "off"
	if m.enabled { enabledStr = "on" }
	s += fmt.Sprintf("  Enabled:  %s (space to toggle)\n", enabledStr)
	s += "\n  enter: save | esc: back"
	return s
}
