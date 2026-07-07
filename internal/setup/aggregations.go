package setup

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"ai-router/internal/config"
)

type aggregationListModel struct {
	cfg    *SetupConfig
	back   *mainMenuModel
	cursor int
}

func newAggregationListModel(cfg *SetupConfig, back *mainMenuModel) aggregationListModel {
	return aggregationListModel{cfg: cfg, back: back}
}

func (m aggregationListModel) Init() tea.Cmd { return nil }

func (m aggregationListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if isBackMsg(msg) { return m.back, nil }
	count := len(m.cfg.Aggregations)
	total := count + 2

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
				return newAggEditModel(m.cfg, &m, -1), nil
			} else if m.cursor == count+1 {
				return m.back, nil
			} else {
				return newAggEditModel(m.cfg, &m, m.cursor), nil
			}
		case "delete", "x":
			if m.cursor < count {
				m.cfg.Aggregations = append(m.cfg.Aggregations[:m.cursor], m.cfg.Aggregations[m.cursor+1:]...)
				if m.cursor >= len(m.cfg.Aggregations) { m.cursor = len(m.cfg.Aggregations) }
			}
		}
	}
	return m, nil
}

func (m aggregationListModel) View() string {
	s := "\n  \033[1mModel Aggregations\033[0m\n\n"
	for i, agg := range m.cfg.Aggregations {
		cursor := "  "
		if m.cursor == i { cursor = "\u25b6 " }
		s += fmt.Sprintf("%s%s [%s] (%d models)\n", cursor, agg.Name, agg.Strategy, len(agg.Models))
	}
	s += "  \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\n"
	addCursor := "  "
	if m.cursor == len(m.cfg.Aggregations) { addCursor = "\u25b6 " }
	s += addCursor + "Add new aggregation\n"
	backCursor := "  "
	if m.cursor == len(m.cfg.Aggregations)+1 { backCursor = "\u25b6 " }
	s += backCursor + "Back\n"
	s += "\n  enter: edit | delete/x: remove | esc: back"
	return s
}

// --- Edit Aggregation ---

type aggEditModel struct {
	cfg      *SetupConfig
	back     tea.Model
	idx      int // -1 = new
	name     textinput.Model
	strategy int // 0=weighted, 1=fallback, 2=round_robin
	models   []config.ModelAggEntry
	cursor   int
	phase    string // "name", "strategy", "models", "addmodel"
	addMdl   textinput.Model
	addWt    textinput.Model
	addProv  int
}

func newAggEditModel(cfg *SetupConfig, back tea.Model, idx int) aggEditModel {
	n := textinput.New()
	strategy := 0
	var models []config.ModelAggEntry
	if idx >= 0 {
		a := cfg.Aggregations[idx]
		n.SetValue(a.Name)
		models = make([]config.ModelAggEntry, len(a.Models))
		copy(models, a.Models)
		switch a.Strategy {
		case "fallback": strategy = 1
		case "round_robin": strategy = 2
		}
	}
	n.Focus()
	a := textinput.New()
	a.SetValue("100")
	return aggEditModel{
		cfg: cfg, back: back, idx: idx,
		name: n, strategy: strategy, models: models,
		phase: "name", addMdl: textinput.New(), addWt: a,
	}
}

func (m aggEditModel) Init() tea.Cmd { return textinput.Blink }

func (m aggEditModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.phase == "addmodel" { m.phase = "models"; return m, nil }
			if m.phase == "models" { m.phase = "strategy"; return m, nil }
			return m.back, nil
		case "enter":
			switch m.phase {
			case "name":
				m.phase = "strategy"
			case "strategy":
				m.phase = "models"
			case "models":
				// Save aggregation
				strats := []string{"weighted", "fallback", "round_robin"}
				agg := config.ModelAggregation{
					Name:     strings.TrimSpace(m.name.Value()),
					Strategy: strats[m.strategy],
					Models:   m.models,
				}
				if m.idx >= 0 {
					m.cfg.Aggregations[m.idx] = agg
				} else {
					m.cfg.Aggregations = append(m.cfg.Aggregations, agg)
				}
				return m.back, nil
			case "addmodel":
				w := 10
				if v, err := strconv.Atoi(m.addWt.Value()); err == nil { w = v }
				m.models = append(m.models, config.ModelAggEntry{
					Provider: m.cfg.Providers[m.addProv].Name,
					Model:    strings.TrimSpace(m.addMdl.Value()),
					Weight:   w,
				})
				m.phase = "models"
			}
		case "up", "k":
			if m.phase == "strategy" && m.strategy > 0 { m.strategy-- }
			if m.phase == "models" && m.cursor > 0 { m.cursor-- }
			if m.phase == "addmodel" && m.addProv > 0 { m.addProv-- }
		case "down", "j":
			if m.phase == "strategy" && m.strategy < 2 { m.strategy++ }
			if m.phase == "models" { m.cursor++ }
			if m.phase == "addmodel" && m.addProv < len(m.cfg.Providers)-1 { m.addProv++ }
		case "a":
			if m.phase == "models" { m.phase = "addmodel"; m.addMdl.SetValue(""); m.addWt.SetValue("100") }
		case "d":
			if m.phase == "models" && m.cursor < len(m.models) {
				m.models = append(m.models[:m.cursor], m.models[m.cursor+1:]...)
				if m.cursor >= len(m.models) { m.cursor = len(m.models)-1 }
				if m.cursor < 0 { m.cursor = 0 }
			}
		}
	}

	if m.phase == "addmodel" {
		var cmd tea.Cmd
		m.addMdl, cmd = m.addMdl.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m aggEditModel) View() string {
	s := "\n  \033[1mEdit Aggregation\033[0m\n\n"
	s += "  Name:     " + m.name.View() + "\n"
	strats := []string{"weighted", "fallback", "round_robin"}
	for i, st := range strats {
		cursor := "  "
		if m.strategy == i { cursor = "\u25b6 " }
		s += cursor + st + "\n"
	}
	s += "\n  Models:\n"
	for i, mdl := range m.models {
		cursor := "  "
		if m.phase == "models" && m.cursor == i { cursor = "\u25b6 " }
		s += fmt.Sprintf("%s  %s / %s (w:%d)\n", cursor, mdl.Provider, mdl.Model, mdl.Weight)
	}
	if m.phase == "addmodel" {
		s += "\n  Add model (select provider):\n"
		for i, p := range m.cfg.Providers {
			cursor := "  "
			if m.addProv == i { cursor = "\u25b6 " }
			s += cursor + p.Name + "\n"
		}
		s += "  Model: " + m.addMdl.View() + "\n"
		s += "  Weight: " + m.addWt.View() + "\n"
	}
	s += "\n  a: add model | d: remove | enter: next/save | esc: back"
	return s
}
