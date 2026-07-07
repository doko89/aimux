package setup

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

var (
	sidebarStyle = lipgloss.NewStyle().
			Width(22).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	activeTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFB86C")).
			Bold(true)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6272A4"))

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8F8F2"))

	contentStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#444444")).
			Padding(0, 2).
			Width(60)

	inputLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8F8F2")).
			Width(12).
			Bold(true)

	btnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#50FA7B")).
			Padding(0, 2)

	btnDangerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#FF5555")).
			Padding(0, 2)

	helpText = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4")).
			Italic(true)
)

func Run() {
	cfg := LoadFromExisting()
	p := tea.NewProgram(newAppModel(cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// ─── App Model ────────────────────────────────────────────────────

type appModel struct {
	cfg         *SetupConfig
	activeTab   int
	auth        authPageModel
	agg         aggregatorPageModel
	settings    settingsPageModel
}

func newAppModel(cfg *SetupConfig) appModel {
	return appModel{
		cfg:       cfg,
		activeTab: 0,
		auth:      newAuthPage(cfg),
		agg:       newAggregatorPage(cfg),
		settings:  newSettingsPage(cfg),
	}
}

func (m appModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Always handle window size and quit
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, nil
	case tea.KeyMsg:
		// Ctrl+C always quits
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// Tab switching
		if msg.String() == "tab" && !m.isEditingActive() {
			m.activeTab = (m.activeTab + 1) % 3
			return m, nil
		}
		// Save & quit
		if msg.String() == "s" && !m.isEditingActive() {
			dir, _ := os.Getwd()
			if err := Save(m.cfg, dir); err != nil {
				fmt.Fprintf(os.Stderr, "Save error: %v\n", err)
			}
			return m, tea.Quit
		}
		// Quit
		if msg.String() == "q" && !m.isEditingActive() {
			return m, tea.Quit
		}
	}

	// Route to active page
	var cmd tea.Cmd
	switch m.activeTab {
	case 0:
		m.auth, cmd = m.auth.update(msg, m.cfg)
	case 1:
		m.agg, cmd = m.agg.update(msg, m.cfg)
	case 2:
		m.settings, cmd = m.settings.update(msg, m.cfg)
	}
	return m, cmd
}

func (m appModel) isEditingActive() bool {
	switch m.activeTab {
	case 0:
		return m.auth.editIdx >= 0
	case 1:
		return m.agg.editIdx >= 0
	case 2:
		return m.settings.isEditing()
	}
	return false
}

func (m appModel) View() string {
	tabs := []string{" Auth ", " Aggregator ", " Settings "}
	var sidebar string
	sidebar += "\n  aimux setup\n\n"
	for i, t := range tabs {
		style := inactiveTabStyle
		prefix := "  "
		if i == m.activeTab {
			style = activeTabStyle
			prefix = "▸ "
		}
		sidebar += prefix + style.Render(t) + "\n"
	}
	sidebar += "\n" + helpText.Render("  tab: switch page") + "\n"
	sidebar += helpText.Render("  s:   save & exit") + "\n"
	sidebar += helpText.Render("  q:   quit (outside edit)")

	var content string
	switch m.activeTab {
	case 0:
		content = m.auth.View()
	case 1:
		content = m.agg.View()
	case 2:
		content = m.settings.View()
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebarStyle.Render(sidebar),
		contentStyle.Render(content),
	)
}
