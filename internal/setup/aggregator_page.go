package setup

import (
	"fmt"
	"strconv"
	"strings"

	"ai-router/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

// ─── Aggregator Page ──────────────────────────────────────────────

type aggregatorPageModel struct {
	cfg         *SetupConfig
	focus       int // 0=agg list, 1=name, 2=strategy, 3=model entries, 4=model picker, 5=weight
	editIdx     int // -1 = no editing
	name        textinput.Model
	strategy    int // 0=weighted, 1=fallback, 2=round_robin
	modelList   int
	models      []config.ModelAggEntry
	addPhase    bool // adding a new model entry to aggregation
	pickProv    int  // which provider selected in add model
	pickModel   int  // which model selected in add model
	weightInput textinput.Model
}

func newAggregatorPage(cfg *SetupConfig) aggregatorPageModel {
	n := textinput.New()
	n.Placeholder = "aggregation-name"
	w := textinput.New()
	w.SetValue("50")
	return aggregatorPageModel{
		cfg:         cfg,
		focus:       0,
		editIdx:     -1,
		name:        n,
		weightInput: w,
	}
}

func (m aggregatorPageModel) Init() tea.Cmd { return textinput.Blink }

func (m aggregatorPageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	if m.focus == 5 {
		var cmd tea.Cmd
		m.weightInput, cmd = m.weightInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m aggregatorPageModel) handleKey(msg tea.KeyMsg) (aggregatorPageModel, tea.Cmd) {
	count := len(m.cfg.Aggregations)

	switch msg.String() {
	// ── List navigation (editIdx == -1) ──
	case "up", "k":
		if m.editIdx == -1 && m.modelList > 0 {
			m.modelList--
		} else if m.addPhase && m.focus == 4 && m.pickProv > 0 {
			m.pickProv--
		} else if m.addPhase && m.focus == 5 {
			// weight input, do nothing
		} else if m.editIdx >= 0 && m.focus == 3 && m.modelList > 0 {
			m.modelList--
		}
	case "down", "j":
		if m.editIdx == -1 {
			max := count + 1 // providers + "Add new" row
			if m.modelList < max-1 {
				m.modelList++
			}
		} else if m.addPhase && m.focus == 4 {
			// Provider picker
			if m.pickProv < len(m.cfg.Providers)-1 {
				m.pickProv++
			}
		} else if m.editIdx >= 0 && m.focus == 3 {
			if m.modelList < len(m.models)-1 {
				m.modelList++
			}
		}

	case "tab":
		if m.editIdx >= 0 && !m.addPhase {
			// cycle focus: name -> strategy -> model entries -> (done)
			focuses := []int{1, 2, 3}
			for i, f := range focuses {
				if m.focus == f && i < len(focuses)-1 {
					m.focus = focuses[i+1]
					return m, nil
				}
			}
			// at end -> save aggregation
			m.commitAgg()
			m.editIdx = -1
			m.focus = 0
		} else if m.addPhase {
			// cycle: provider picker -> model picker -> weight
			if m.focus == 4 {
				m.focus = 5
				m.weightInput.Focus()
			} else if m.focus == 5 {
				m.addModelEntry()
				m.focus = 3
			}
		}

	case "enter":
		if m.editIdx == -1 {
			if m.modelList < count {
				// Edit existing
				m.editIdx = m.modelList
				m.loadAgg()
				m.focus = 1
			} else {
				// Add new aggregation
				m.editIdx = count
				m.name.SetValue("")
				m.strategy = 0
				m.models = nil
				m.name.Focus()
				m.focus = 1
				m.cfg.Aggregations = append(m.cfg.Aggregations, config.ModelAggregation{})
			}
		} else if m.addPhase {
			m.addModelEntry()
			m.addPhase = false
			m.focus = 3
		}

	case "a", "A":
		if m.editIdx >= 0 && m.focus == 3 && !m.addPhase {
			m.addPhase = true
			m.focus = 4
			m.pickProv = 0
			m.pickModel = 0
			m.weightInput.SetValue("50")
		}

	case "d", "delete", "x":
		if m.editIdx >= 0 && m.focus == 3 && m.modelList < len(m.models) {
			m.models = append(m.models[:m.modelList], m.models[m.modelList+1:]...)
			if m.modelList >= len(m.models) {
				m.modelList = len(m.models) - 1
			}
			if m.modelList < 0 {
				m.modelList = 0
			}
		} else if m.editIdx == -1 && count > 0 && m.modelList < count {
			m.cfg.Aggregations = append(m.cfg.Aggregations[:m.modelList], m.cfg.Aggregations[m.modelList+1:]...)
			if m.modelList >= len(m.cfg.Aggregations) {
				m.modelList = len(m.cfg.Aggregations) - 1
			}
			if m.modelList < 0 {
				m.modelList = 0
			}
		}

	case "escape", "esc":
		if m.addPhase {
			m.addPhase = false
			m.focus = 3
		} else if m.editIdx >= 0 {
			m.commitAgg()
			m.editIdx = -1
			m.focus = 0
			m.modelList = 0
		}
	}

	// Handle strategy toggle with left/right or numbers
	if m.editIdx >= 0 && m.focus == 2 {
		switch msg.String() {
		case "left", "1":
			if m.strategy > 0 {
				m.strategy--
			}
		case "right", "3":
			if m.strategy < 2 {
				m.strategy++
			}
		case "2":
			m.strategy = 1
		}
	}

	// Handle name input
	if m.editIdx >= 0 && m.focus == 1 {
		var cmd tea.Cmd
		m.name, cmd = m.name.Update(msg)
		return m, cmd
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
		return []string{"(select provider first)"}
	}
	p := m.cfg.Providers[m.pickProv]
	models := p.AvailableModels
	if len(models) == 0 && p.Model != "" {
		models = []string{p.Model}
	}
	if len(models) == 0 {
		models = []string{"(no models available)"}
	}
	return models
}

func (m aggregatorPageModel) View() string {
	s := "\n"

	// Aggregation list
	s += inputLabelStyle.Render("Aggregations") + "\n"
	if len(m.cfg.Aggregations) == 0 {
		s += "  (no aggregations configured)\n"
	} else {
		for i, a := range m.cfg.Aggregations {
			cursor := "  "
			if m.editIdx == -1 && m.modelList == i {
				cursor = "▸ "
			} else if m.editIdx == i {
				cursor = "▸ "
			}
			line := fmt.Sprintf("%s%s [%s] (%d models)", cursor, a.Name, a.Strategy, len(a.Models))
			if m.editIdx == i {
				s += activeTabStyle.Render(line) + "\n"
			} else {
				s += normalStyle.Render(line) + "\n"
			}
		}
	}

	// "Add new" row
	addCursor := "  "
	if m.editIdx == -1 && m.modelList == len(m.cfg.Aggregations) {
		addCursor = "▸ "
	}
	s += addCursor + normalStyle.Render("+ Add new aggregation") + "\n\n"

	// Edit form
	if m.editIdx >= 0 {
		s += helpText.Render("── Edit Aggregation ──") + "\n\n"

		// Name
		cursor := "  "
		if m.focus == 1 {
			m.name.Focus()
			cursor = "▸ "
		} else {
			m.name.Blur()
		}
		s += cursor + inputLabelStyle.Render("Name") + m.name.View() + "\n"

		// Strategy
		cursor = "  "
		if m.focus == 2 {
			cursor = "▸ "
		}
		s += cursor + inputLabelStyle.Render("Strategy") + "\n"
		strategies := []string{"weighted", "fallback", "round_robin"}
		for i, st := range strategies {
			prefix := "    "
			if m.strategy == i {
				prefix = "    " + activeTabStyle.Render("● ") + st
			} else {
				prefix = "    ○ " + st
			}
			s += prefix + "\n"
		}

		// Model entries
		s += "\n" + inputLabelStyle.Render("Models") + "\n"
		if len(m.models) == 0 {
			s += "  (no models — press a to add)\n"
		} else {
			for i, me := range m.models {
				cursor := "  "
				if m.focus == 3 && m.modelList == i {
					cursor = "▸ "
				}
				s += fmt.Sprintf("%s%s/%s (w:%d)\n", cursor, me.Provider, me.Model, me.Weight)
			}
		}

		// Add model picker
		if m.addPhase {
			s += "\n" + helpText.Render("── Add Model ──") + "\n"
			// Provider picker
			s += inputLabelStyle.Render("Provider") + "\n"
			for i, p := range m.cfg.Providers {
				cursor := "  "
				if m.focus == 4 && m.pickProv == i {
					cursor = "▸ "
					s += cursor + activeTabStyle.Render(p.Name) + "\n"
				} else {
					s += cursor + normalStyle.Render(p.Name) + "\n"
				}
			}
			// Model picker
			modelChoices := m.getModelChoices()
			s += inputLabelStyle.Render("Model") + "\n"
			for i, mc := range modelChoices {
				cursor := "  "
				if m.focus == 5 {
					// weight input is active, show selected model
					if i == m.pickModel {
						cursor = "▸ "
						s += cursor + activeTabStyle.Render(mc) + "\n"
					} else {
						s += cursor + normalStyle.Render(mc) + "\n"
					}
				} else {
					s += cursor + normalStyle.Render(mc) + "\n"
				}
			}
			// Weight
			s += "\n" + inputLabelStyle.Render("Weight") + m.weightInput.View() + "\n"
			s += helpText.Render("enter: add model | esc: cancel") + "\n"
		}

		if !m.addPhase {
			s += "\n" + helpText.Render("a: add model | d/x: remove model | enter/done: save | esc: back") + "\n"
		}
	} else {
		s += helpText.Render("enter: edit | a: add new | d/x: remove | s: save all") + "\n"
	}

	return s
}
