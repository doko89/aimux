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
	focus       int // 1=name, 2=strategy, 3=models, 4=addModelBtn, 5=saveBtn
	strategy    int
	modelList   int
	models      []config.ModelAggEntry
	addPhase    bool
	pickProv    int
	pickModel   int
	pickWeight  textinput.Model
	name        textinput.Model
	aggList     int
	// Strategy modal
	showStrategy bool
	stratCursor  int
	// Add model modal
	showAddModal    bool
	addCursor       int
	searchInput     textinput.Model
	searchIdx       int
}

func newAggregatorPage(cfg *SetupConfig) aggregatorPageModel {
	return aggregatorPageModel{
		cfg:         cfg,
		editIdx:     -1,
		name:        mkInput("", "aggregation-name"),
		pickWeight:  mkInput("50", "weight"),
		searchInput: mkInput("", "search model..."),
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

	// Strategy modal has priority
	if m.showStrategy {
		return m.updateStrategyModal(key)
	}

	// Add model modal has priority
	if m.showAddModal {
		return m.updateAddModal(key, keyMsg)
	}

	if m.editIdx >= 0 {
		updated, cmd := m.updateEdit(keyMsg, key)
		return updated, cmd
	}

	return m.updateList(key)
}

// ── LIST MODE ──

func (m aggregatorPageModel) updateList(key string) (aggregatorPageModel, tea.Cmd) {
	count := len(m.cfg.Aggregations)
	total := count + 1

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
			m.blurAll()
		} else {
			m.editIdx = count
			m.cfg.Aggregations = append(m.cfg.Aggregations, config.ModelAggregation{})
			m.name.SetValue("")
			m.strategy = 0
			m.models = nil
			m.focus = 1
			m.blurAll()
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
	case "enter":
		switch m.focus {
		case 1:
			m.focus = 2
		case 2:
			// Open strategy modal
			m.showStrategy = true
			m.stratCursor = m.strategy
		case 3:
			m.focus = 4
		case 4:
			// Open add model modal
			m.showAddModal = true
			m.addCursor = 0
			m.searchInput.SetValue("")
			m.searchInput.Focus()
			m.pickModel = 0
		case 5:
			m.commitAgg()
			m.editIdx = -1
			m.focus = 0
		}
		m.blurAll()
		return m, nil
	}

	if m.focus == 1 {
		m.name, _ = m.name.Update(msg)
	}
	return m, nil
}

// ── STRATEGY MODAL ──

func (m aggregatorPageModel) updateStrategyModal(key string) (aggregatorPageModel, tea.Cmd) {
	strategies := []string{"weighted", "fallback", "round_robin"}
	switch key {
	case "up", "k":
		if m.stratCursor > 0 {
			m.stratCursor--
		}
	case "down", "j":
		if m.stratCursor < len(strategies)-1 {
			m.stratCursor++
		}
	case "enter":
		m.strategy = m.stratCursor
		m.showStrategy = false
	case "esc":
		m.showStrategy = false
	}
	return m, nil
}

// ── ADD MODEL MODAL ──
// Shows flat list of all provider/model pairs with search

type providerModelItem struct {
	provider string
	model    string
}

func (m aggregatorPageModel) allProviderModels() []providerModelItem {
	var items []providerModelItem
	for _, p := range m.cfg.Providers {
		models := p.AvailableModels
		if len(models) == 0 && p.Model != "" {
			models = []string{p.Model}
		}
		for _, mdl := range models {
			items = append(items, providerModelItem{provider: p.Name, model: mdl})
		}
	}
	return items
}

func (m aggregatorPageModel) filteredItems() []providerModelItem {
	query := strings.ToLower(m.searchInput.Value())
	items := m.allProviderModels()
	if query == "" {
		return items
	}
	var filtered []providerModelItem
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.provider), query) ||
			strings.Contains(strings.ToLower(item.model), query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (m aggregatorPageModel) updateAddModal(key string, msg tea.KeyMsg) (aggregatorPageModel, tea.Cmd) {
	switch key {
	case "esc":
		m.showAddModal = false
		m.focus = 4
		return m, nil
	case "up", "k":
		if m.addCursor > 0 {
			m.addCursor--
		}
		return m, nil
	case "down", "j":
		items := m.filteredItems()
		if m.addCursor < len(items)-1 {
			m.addCursor++
		}
		return m, nil
	case "enter":
		items := m.filteredItems()
		if m.addCursor < len(items) {
			item := items[m.addCursor]
			w := 50
			if v, err := strconv.Atoi(m.pickWeight.Value()); err == nil {
				w = v
			}
			m.models = append(m.models, config.ModelAggEntry{
				Provider: item.provider,
				Model:    item.model,
				Weight:   w,
			})
			m.showAddModal = false
			m.focus = 3
		}
		return m, nil
	}

	// Route typing to search input
	m.searchInput, _ = m.searchInput.Update(msg)
	m.addCursor = 0 // reset cursor when search changes
	return m, nil
}

// ── HELPERS ──

func (m *aggregatorPageModel) blurAll() {
	m.name.Blur()
	m.pickWeight.Blur()
	m.searchInput.Blur()
	if m.focus == 1 {
		m.name.Focus()
	}
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

	// Modals overlay on top
	if m.showStrategy {
		s = m.renderStrategyModal()
	}
	if m.showAddModal {
		s = m.renderAddModal()
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
	s += cursor + inputLabelStyle.Render("Strategy") + "  enter to select\n"
	strategies := []string{"weighted", "fallback", "round_robin"}
	mark := "○"
	if m.focus == 2 {
		mark = "●"
	}
	s += fmt.Sprintf("      %s %s\n", mark, strategies[m.strategy])

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

	s += "\n"
	s += m.renderButton("Add Model", 4, false) + "  "
	s += m.renderButton("Save", 5, false)
	s += "\n"

	return s
}

func (m aggregatorPageModel) renderStrategyModal() string {
	strategies := []string{"weighted", "fallback", "round_robin"}
	s := helpText.Render("── Select Strategy ──") + "\n\n"
	for i, st := range strategies {
		cursor := "  "
		if m.stratCursor == i {
			cursor = "▸ "
			s += cursor + activeTabStyle.Render(st) + "\n"
		} else {
			s += cursor + normalStyle.Render(st) + "\n"
		}
	}
	s += "\n" + helpText.Render("↑↓: select | enter: confirm | esc: cancel")
	return s
}

func (m aggregatorPageModel) renderAddModal() string {
	s := helpText.Render("── Add Model ──") + "\n\n"
	s += "  " + m.searchInput.View() + "\n\n"

	items := m.filteredItems()
	if len(items) == 0 {
		s += "  (no models found)\n"
	} else {
		// Show max 10 items
		maxShow := 10
		if len(items) < maxShow {
			maxShow = len(items)
		}
		for i := 0; i < maxShow; i++ {
			item := items[i]
			cursor := "  "
			if m.addCursor == i {
				cursor = "▸ "
				s += cursor + activeTabStyle.Render(item.provider+"/"+item.model) + "\n"
			} else {
				s += cursor + normalStyle.Render(item.provider+"/"+item.model) + "\n"
			}
		}
		if len(items) > maxShow {
			s += helpText.Render(fmt.Sprintf("  ... %d more", len(items)-maxShow)) + "\n"
		}
	}

	s += "\n" + inputLabelStyle.Render("Weight") + m.pickWeight.View() + "\n"
	s += "\n" + helpText.Render("↑↓: select | enter: add | esc: cancel")
	return s
}

func (m aggregatorPageModel) renderButton(label string, focus int, highlight bool) string {
	if highlight {
		return activeTabStyle.Render("▸ [" + label + "]")
	}
	if m.editIdx >= 0 && m.focus == focus {
		return activeTabStyle.Render("▸ [" + label + "]")
	}
	return normalStyle.Render("  [" + label + "]")
}
