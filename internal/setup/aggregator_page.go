package setup

import (
	"fmt"
	"strconv"
	"strings"

	"ai-router/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type aggregatorPageModel struct {
	cfg         *SetupConfig
	editIdx     int
	focus       int // 0=list, 1=name, 2=strategy, 3=models, 4=pickProv, 5=pickModel, 6=weight
	strategy    int
	modelList   int
	models      []config.ModelAggEntry
	addPhase    bool
	pickProv    int
	pickModel   int
	name        textinput.Model
	weightInput textinput.Model
}

func newAggregatorPage(cfg *SetupConfig) aggregatorPageModel {
	return aggregatorPageModel{
		cfg:         cfg,
		editIdx:     -1,
		name:        mkInput("", "aggregation-name"),
		weightInput: mkInput("50", "50"),
	}
}

func (m aggregatorPageModel) Init() tea.Cmd { return textinput.Blink }

func (m aggregatorPageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	new, cmd := m.update(msg, m.cfg)
	return new, cmd
}

func (m aggregatorPageModel) update(msg tea.Msg, cfg *SetupConfig) (aggregatorPageModel, tea.Cmd) {
	m.cfg = cfg

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	key := keyMsg.String()

	// Route to text input when editing
	if m.editIdx >= 0 && !m.addPhase && m.focus == 1 {
		m.name, _ = m.name.Update(msg)
	}
	if m.addPhase && m.focus == 6 {
		m.weightInput, _ = m.weightInput.Update(msg)
	}

	count := len(m.cfg.Aggregations)

	// ── ADD MODEL PHASE ──
	if m.addPhase {
		return m.updateAddPhase(key, keyMsg)
	}

	// ── EDIT MODE ──
	if m.editIdx >= 0 {
		return m.updateEdit(key, count)
	}

	// ── LIST MODE ──
	return m.updateList(key, count)
}

func (m aggregatorPageModel) updateList(key string, count int) (aggregatorPageModel, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.modelList > 0 {
			m.modelList--
		}
	case "down", "j":
		if m.modelList < count {
			m.modelList++
		}
	case "enter":
		if m.modelList < count {
			m.editIdx = m.modelList
			m.loadAgg()
			m.focus = 1
		} else {
			m.editIdx = count
			m.cfg.Aggregations = append(m.cfg.Aggregations, config.ModelAggregation{})
			m.name.SetValue("")
			m.strategy = 0
			m.models = nil
			m.focus = 1
		}
	case "a":
		m.editIdx = count
		m.cfg.Aggregations = append(m.cfg.Aggregations, config.ModelAggregation{})
		m.name.SetValue("")
		m.strategy = 0
		m.models = nil
		m.focus = 1
	case "d", "delete":
		if count > 0 && m.modelList < count {
			m.cfg.Aggregations = append(m.cfg.Aggregations[:m.modelList], m.cfg.Aggregations[m.modelList+1:]...)
			if m.modelList >= len(m.cfg.Aggregations) && m.modelList > 0 {
				m.modelList--
			}
		}
	}
	return m, nil
}

func (m aggregatorPageModel) updateEdit(key string, count int) (aggregatorPageModel, tea.Cmd) {
	switch key {
	case "esc":
		m.commitAgg()
		m.editIdx = -1
		m.focus = 0
		m.modelList = 0
	case "tab":
		switch m.focus {
		case 1:
			m.focus = 2
		case 2:
			m.focus = 3
		case 3:
			m.commitAgg()
			m.editIdx = -1
			m.focus = 0
		}
	case "shift+tab":
		if m.focus > 1 {
			m.focus--
		}
	case "a":
		if m.focus == 3 {
			m.addPhase = true
			m.focus = 4
			m.pickProv = 0
			m.pickModel = 0
			m.weightInput.SetValue("50")
		}
	case "d", "x":
		if m.focus == 3 && m.modelList < len(m.models) {
			m.models = append(m.models[:m.modelList], m.models[m.modelList+1:]...)
			if m.modelList >= len(m.models) && m.modelList > 0 {
				m.modelList--
			}
		}
	case "left":
		if m.focus == 2 && m.strategy > 0 {
			m.strategy--
		}
	case "right":
		if m.focus == 2 && m.strategy < 2 {
			m.strategy++
		}
	case "up", "k":
		if m.focus == 3 && m.modelList > 0 {
			m.modelList--
		}
	case "down", "j":
		if m.focus == 3 && m.modelList < len(m.models)-1 {
			m.modelList++
		}
	}
	return m, nil
}

func (m aggregatorPageModel) updateAddPhase(key string, msg tea.KeyMsg) (aggregatorPageModel, tea.Cmd) {
	modelChoices := m.getModelChoices()
	switch key {
	case "esc":
		m.addPhase = false
		m.focus = 3
	case "up", "k":
		if m.focus == 4 && m.pickProv > 0 {
			m.pickProv--
			m.pickModel = 0
		} else if m.focus == 5 && m.pickModel > 0 {
			m.pickModel--
		}
	case "down", "j":
		if m.focus == 4 && m.pickProv < len(m.cfg.Providers)-1 {
			m.pickProv++
			m.pickModel = 0
		} else if m.focus == 5 && m.pickModel < len(modelChoices)-1 {
			m.pickModel++
		}
	case "tab":
		switch m.focus {
		case 4:
			m.focus = 5
		case 5:
			m.focus = 6
		case 6:
			m.addModelEntry()
			m.addPhase = false
			m.focus = 3
		}
	case "enter":
		m.addModelEntry()
		m.addPhase = false
		m.focus = 3
	}
	return m, nil
}

func (m *aggregatorPageModel) loadAgg() {
	if m.editIdx < 0 || m.editIdx >= len(m.cfg.Aggregations) {
		return
	}
	a := m.cfg.Aggregations[m.editIdx]
	m.name.SetValue(a.Name)
	m.strategy = 0
	switch a.Strategy {
	case "fallback":
		m.strategy = 1
	case "round_robin":
		m.strategy = 2
	}
	m.models = make([]config.ModelAggEntry, len(a.Models))
	copy(m.models, a.Models)
	m.modelList = 0
}

func (m *aggregatorPageModel) commitAgg() {
	if m.editIdx < 0 || m.editIdx >= len(m.cfg.Aggregations) {
		return
	}
	strategies := []string{"weighted", "fallback", "round_robin"}
	m.cfg.Aggregations[m.editIdx] = config.ModelAggregation{
		Name:     strings.TrimSpace(m.name.Value()),
		Strategy: strategies[m.strategy],
		Models:   m.models,
	}
}

func (m *aggregatorPageModel) addModelEntry() {
	if m.pickProv >= len(m.cfg.Providers) {
		return
	}
	p := m.cfg.Providers[m.pickProv]
	models := p.AvailableModels
	if len(models) == 0 && p.Model != "" {
		models = []string{p.Model}
	}
	if len(models) == 0 {
		return
	}
	if m.pickModel >= len(models) {
		m.pickModel = 0
	}
	w := 50
	if v, err := strconv.Atoi(m.weightInput.Value()); err == nil {
		w = v
	}
	m.models = append(m.models, config.ModelAggEntry{
		Provider: p.Name,
		Model:    models[m.pickModel],
		Weight:   w,
	})
}

func (m aggregatorPageModel) getModelChoices() []string {
	if m.pickProv >= len(m.cfg.Providers) {
		return []string{}
	}
	p := m.cfg.Providers[m.pickProv]
	models := p.AvailableModels
	if len(models) == 0 && p.Model != "" {
		models = []string{p.Model}
	}
	return models
}

func (m aggregatorPageModel) View() string {
	s := "\n" + inputLabelStyle.Render("Aggregations") + "\n\n"

	count := len(m.cfg.Aggregations)
	if count == 0 {
		s += "  (none)\n"
	} else {
		for i, a := range m.cfg.Aggregations {
			cursor := "  "
			if m.editIdx == i || (m.editIdx == -1 && m.modelList == i) {
				cursor = "▸ "
			}
			line := fmt.Sprintf("%s%s [%s] (%d)", cursor, a.Name, a.Strategy, len(a.Models))
			if m.editIdx == i {
				s += activeTabStyle.Render(line) + "\n"
			} else {
				s += normalStyle.Render(line) + "\n"
			}
		}
	}

	addCursor := "  "
	if m.editIdx == -1 && m.modelList == count {
		addCursor = "▸ "
	}
	s += addCursor + normalStyle.Render("+ Add new") + "\n\n"

	if m.editIdx >= 0 {
		s += m.renderEdit()
	} else {
		s += helpText.Render("enter: edit | a: add | d: remove")
	}

	return s
}

func (m aggregatorPageModel) renderEdit() string {
	s := helpText.Render("── Edit Aggregation ──") + "\n\n"

	cursor := "  "
	if m.focus == 1 {
		cursor = "▸ "
	}
	s += cursor + inputLabelStyle.Render("Name") + m.name.View() + "\n\n"

	cursor = "  "
	if m.focus == 2 {
		cursor = "▸ "
	}
	s += cursor + inputLabelStyle.Render("Strategy") + "\n"
	strategies := []string{"weighted", "fallback", "round_robin"}
	for i, st := range strategies {
		mark := "○"
		if m.strategy == i {
			mark = "●"
		}
		s += fmt.Sprintf("      %s %s\n", mark, st)
	}

	s += "\n" + inputLabelStyle.Render("Models") + "\n"
	if len(m.models) == 0 {
		s += "  (empty — press a)\n"
	} else {
		for i, me := range m.models {
			cursor := "  "
			if m.focus == 3 && m.modelList == i {
				cursor = "▸ "
			}
			s += fmt.Sprintf("%s%s/%s (w:%d)\n", cursor, me.Provider, me.Model, me.Weight)
		}
	}

	if m.addPhase {
		s += "\n" + helpText.Render("── Pick Model ──") + "\n"
		s += inputLabelStyle.Render("Provider") + "\n"
		for i, p := range m.cfg.Providers {
			c := "  "
			if m.focus == 4 && m.pickProv == i {
				c = "▸ "
				s += c + activeTabStyle.Render(p.Name) + "\n"
			} else {
				s += c + normalStyle.Render(p.Name) + "\n"
			}
		}
		choices := m.getModelChoices()
		s += inputLabelStyle.Render("Model") + "\n"
		for i, mc := range choices {
			c := "  "
			if m.focus == 5 && m.pickModel == i {
				c = "▸ "
				s += c + activeTabStyle.Render(mc) + "\n"
			} else {
				s += c + normalStyle.Render(mc) + "\n"
			}
		}
		cursor = "  "
		if m.focus == 6 {
			cursor = "▸ "
		}
		s += "\n" + cursor + inputLabelStyle.Render("Weight") + m.weightInput.View() + "\n"
		s += helpText.Render("enter/tab: add | esc: cancel")
	} else {
		s += "\n" + helpText.Render("a: add model | d: remove | tab/done: save | esc: back")
	}

	return s
}
