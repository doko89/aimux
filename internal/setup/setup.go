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
			BorderForeground(lipgloss.Color("#444444")).
			Padding(0, 1)

	contentStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#444444")).
			Padding(0, 2).
			Width(60)

	activeTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFB86C")).
			Bold(true)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6272A4"))

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8F8F2"))

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

	focusColor = lipgloss.Color("#7D56F4")
)

func Run() {
	cfg := LoadFromExisting()
	p := tea.NewProgram(newAppModel(cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

type appModel struct {
	cfg           *SetupConfig
	sidebarFocus  bool
	activeTab     int // 0=auth, 1=agg, 2=settings
	sidebarCursor int
	auth          authPageModel
	agg           aggregatorPageModel
	settings      settingsPageModel
}

func newAppModel(cfg *SetupConfig) appModel {
	return appModel{
		cfg:          cfg,
		sidebarFocus: true,
		activeTab:    0,
		auth:         newAuthPage(cfg),
		agg:          newAggregatorPage(cfg),
		settings:     newSettingsPage(cfg),
	}
}

func (m appModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		// Non-key messages (async results) → route to active page
		return m.routeToPage(msg)
	}

	key := keyMsg.String()

	// Global keys — always handled
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.sidebarFocus = !m.sidebarFocus
		return m, nil
	case "s":
		dir, _ := os.Getwd()
		if err := Save(m.cfg, dir); err != nil {
			fmt.Fprintf(os.Stderr, "Save error: %v\n", err)
		}
		return m, tea.Quit
	case "q":
		return m, tea.Quit
	}

	// Route by focus
	if m.sidebarFocus {
		return m.updateSidebar(key)
	}
	return m.routeToPage(keyMsg)
}

func (m appModel) updateSidebar(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.sidebarCursor > 0 {
			m.sidebarCursor--
			m.activeTab = m.sidebarCursor
		}
	case "down", "j":
		if m.sidebarCursor < 2 {
			m.sidebarCursor++
			m.activeTab = m.sidebarCursor
		}
	case "enter":
		m.sidebarFocus = false
	}
	return m, nil
}

func (m appModel) routeToPage(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	if m.sidebarFocus {
		sidebar += "\n" + helpText.Render("  ↑↓: navigate") + "\n"
		sidebar += helpText.Render("  enter: select") + "\n"
		sidebar += helpText.Render("  tab: content") + "\n"
		sidebar += helpText.Render("  s: save | q: quit")
	} else {
		sidebar += "\n" + helpText.Render("  tab: sidebar") + "\n"
		sidebar += helpText.Render("  ↑↓: navigate fields") + "\n"
		sidebar += helpText.Render("  enter: action/next") + "\n"
		sidebar += helpText.Render("  s: save | q: quit")
	}

	var content string
	switch m.activeTab {
	case 0:
		content = m.auth.View()
	case 1:
		content = m.agg.View()
	case 2:
		content = m.settings.View()
	}

	// Border highlight
	sb := sidebarStyle
	ct := contentStyle
	if m.sidebarFocus {
		sb = sb.BorderForeground(focusColor)
	} else {
		ct = ct.BorderForeground(focusColor)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, sb.Render(sidebar), ct.Render(content))
}
