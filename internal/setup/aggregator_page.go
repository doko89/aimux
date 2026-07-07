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
	focus       int // edit: 1=name, 2=strategy, 3=models, 4=addModelBtn, 5=saveBtn
	strategy    int
	modelList   int
	models      []config.ModelAggEntry
	addPhase    bool
	pickProv    int
	pickModel   int
	weightInput textinput.Model
	name        textinput.Model
	aggList     int
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

	if m.addPhase {
		return m.updateAddPhase(key, keyMsg)
	}

	if m.editIdx >= 0 {
		updated, cmd := m.updateEdit(keyMsg, key)
		return updated, cmd
	}

	return m.updateList(key)
}

// ── LIST MODE ──
// Aggregations + [Add] button

func (m aggregatorPageModel) updateList(key string) (aggregatorPageModel, tea.Cmd) {
	count := len(m.cfg.Aggregations)
	addRow := count
	total := addRow + 1

	switch key {
	case "up", "k":
		if m.aggList > 0 {
			m.aggList--
		}
	case "down", "j":
		if m.aggList < total-1 {
			m.aggList++
		}
	case "enter":
		if m.aggList < count {
			m.editIdx = m.aggList
			m.loadAgg()
			m.focus = 1
		} else if m.aggList == addRow {
			m.editIdx = count
			m.cfg.Aggregations = append(m.cfg.Aggregations, config.ModelAggregation{})
			m.name.SetValue("")
			m.strategy = 0
			m.models = nil
			m.focus = 1
		}
	case "d", "delete":
		if count > 0 && m.aggList < count {
			m.cfg.Aggregations = append(m.cfg.Aggregations[:m.aggList], m.cfg.Aggregations[m.aggList+1:]...)
			if m.aggList >= len(m.cfg.Aggregations) && m.aggList > 0 {
				m.aggList--
			}
		}
	}
	return m, nil
}

// ── EDIT MODE ──
// Name, Strategy, Models, [Add Model], [Save]

func (m aggregatorPageModel) updateEdit(msg tea.KeyMsg, key string) (aggregatorPageModel, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.focus > 1 {
			m.focus--
		}
		m.blurAll()
		return m, nil
	case "down", "j":
		if m.focus < 5 {
			m.focus++
		}
		m.blurAll()
		return m, nil
	case "esc":
		m.commitAgg()
		m.editIdx = -1
		m.focus = 0
		return m, nil
	case "left":
		if m.focus == 2 && m.strategy > 0 {
			m.strategy--
		}
		return m, nil
	case "right":
		if m.focus == 2 && m.strategy < 2 {
			m.strategy++
		}
		return m, nil
	case "enter":
		switch m.focus {
		case 1:
			m.focus = 2
		case 2:
			m.focus = 3
		case 3:
			// Models list selected — nothing special, move to add model btn
			m.focus = 4
		case 4:
			// Add Model button
			m.addPhase = true
			m.pickProv = 0
			m.pickModel = 0
			m.weightInput.SetValue("50")
		case 5:
			// Save button
			m.commitAgg()
			m.editIdx = -1
			m.focus = 0
		}
		m.blurAll()
		return m, nil
	}

	// Typing → name input
	if m.focus == 1 {
		m.name, _ = m.name.Update(msg)
	}
	return m, nil
}

func (m *aggregatorPageModel) blurAll() {
	m.name.Blur()
	m.weightInput.Blur()
	if m.focus == 1 {
		m.name.Focus()
	}
}

// ── ADD MODEL PHASE ──
// Provider picker, Model picker, Weight input

func (m aggregatorPageModel) updateAddPhase(key string, msg tea.KeyMsg) (aggregatorPageModel, tea.Cmd) {
	modelChoices := m.getModelChoices()
	switch key {
	case "up", "k":
		if m.focus == 4 && m.pickProv > 0 {
			m.pickProv--
			m.pickModel = 0
		} else if m.focus == 5 && m.pickModel > 0 {
			m.pickModel--
		}
		return m, nil
	case "down", "j":
		if m.focus == 4 && m.pickProv < len(m.cfg.Providers)-1 {
			m.pickProv++
			m.pickModel = 0
		} else if m.focus == 5 && m.pickModel < len(modelChoices)-1 {
			m.pickModel++
		}
		return m, nil
	case "esc":
		m.addPhase = false
		m.focus = 4
		return m, nil
	case "enter":
		m.addModelEntry()
		m.addPhase = false
		m.focus = 3
		return m, nil
	}

	// Typing → weight input
	if m.focus == 6 {
		m.weightInput, _ = m.weightInput.Update(msg)
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

// ── VIEW ──

func (m aggregatorPageModel) View() string {
	s := "\n" + inputLabelStyle.Render("Aggregations") + "\n\n"

	count := len(m.cfg.Aggregations)
	if count == 0 {
		s += "  (none)\n"
	} else {
		for i, a := range m.cfg.Aggregations {
			cursor := "  "
			if m.editIdx == -1 && m.aggList == i {
				cursor = "▸ "
			} else if m.editIdx == i {
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

	// [Add] button in list mode
	if m.editIdx < 0 {
		s += "\n"
		s += m.renderButton("Add", 0, m.aggList == count)
		s += "\n"
	}

	s += "\n"

	if m.editIdx >= 0 {
		s += m.renderEdit()
	} else {
		s += helpText.Render("enter: edit/add | d: remove")
	}

	return s
}

func (m aggregatorPageModel) renderEdit() string {
	s := helpText.Render("── Edit Aggregation ──") + "\n\n"

	// Name
	cursor := "  "
	if m.focus == 1 {
		cursor = "▸ "
	}
	s += cursor + inputLabelStyle.Render("Name") + m.name.View() + "\n\n"

	// Strategy
	cursor = "  "
	if m.focus == 2 {
		cursor = "▸ "
	}
	s += cursor + inputLabelStyle.Render("Strategy") + "  (←→ to change)\n"
	strategies := []string{"weighted", "fallback", "round_robin"}
	for i, st := range strategies {
		mark := "○"
		if m.strategy == i {
			mark = "●"
		}
		s += fmt.Sprintf("      %s %s\n", mark, st)
	}

	// Models
	s += "\n" + inputLabelStyle.Render("Models") + "\n"
	if len(m.models) == 0 {
		s += "  (empty)\n"
	} else {
		for i, me := range m.models {
			cursor := "  "
			if m.focus == 3 && m.modelList == i {
				cursor = "▸ "
			}
			s += fmt.Sprintf("%s%s/%s (w:%d)\n", cursor, me.Provider, me.Model, me.Weight)
		}
	}

	// Buttons
	s += "\n"
	s += m.renderButton("Add Model", 4, false) + "  "
	s += m.renderButton("Save", 5, false)
	s += "\n"

	// Add model picker
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
		s += helpText.Render("enter: add | esc: cancel")
	}

	return s
}

func (m aggregatorPageModel) renderButton(label string, focus int, highlight bool) string {
	if m.focus == focus || highlight {
		return activeTabStyle.Render("▸ [" + label + "]")
	}
	return normalStyle.Render("  [" + label + "]")
}
