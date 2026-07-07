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
	cfg            *SetupConfig
	sidebarFocus   bool // true=sidebar focused, false=content focused
	activeTab      int  // 0=auth, 1=agg, 2=settings
	sidebarCursor  int  // selected item in sidebar
	auth           authPageModel
	agg            aggregatorPageModel
	settings       settingsPageModel
}

func newAppModel(cfg *SetupConfig) appModel {
	return appModel{
		cfg:           cfg,
		sidebarFocus:  true,
		activeTab:     0,
		sidebarCursor: 0,
		auth:          newAuthPage(cfg),
		agg:           newAggregatorPage(cfg),
		settings:      newSettingsPage(cfg),
	}
}

func (m appModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	key := keyMsg.String()

	// Ctrl+C always quits
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	// ── Sidebar focused ──
	if m.sidebarFocus {
		return m.updateSidebar(key)
	}

	// ── Content focused ──
	return m.updateContent(keyMsg, key)
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
	case "tab":
		// Switch to content area
		m.sidebarFocus = false
	case "enter":
		// Also switch to content on enter
		m.sidebarFocus = false
	case "s":
		dir, _ := os.Getwd()
		if err := Save(m.cfg, dir); err != nil {
			fmt.Fprintf(os.Stderr, "Save error: %v\n", err)
		}
		return m, tea.Quit
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m appModel) updateContent(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab":
		// Switch back to sidebar
		m.sidebarFocus = true
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

	// Help text changes based on focus
	if m.sidebarFocus {
		sidebar += "\n" + helpText.Render("  ↑↓: navigate") + "\n"
		sidebar += helpText.Render("  tab: content") + "\n"
		sidebar += helpText.Render("  s:   save") + "\n"
		sidebar += helpText.Render("  q:   quit")
	} else {
		sidebar += "\n" + helpText.Render("  tab: sidebar") + "\n"
		sidebar += helpText.Render("  ↑↓: navigate fields") + "\n"
		sidebar += helpText.Render("  s:   save") + "\n"
		sidebar += helpText.Render("  q:   quit")
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

	// Dynamic border color based on focus
	sidebarBorder := lipgloss.Color("#444444")
	contentBorder := lipgloss.Color("#444444")
	if m.sidebarFocus {
		sidebarBorder = lipgloss.Color("#7D56F4")
	} else {
		contentBorder = lipgloss.Color("#7D56F4")
	}

	sb := sidebarStyle.BorderForeground(sidebarBorder)
	ct := contentStyle.BorderForeground(contentBorder)

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		sb.Render(sidebar),
		ct.Render(content),
	)
}
