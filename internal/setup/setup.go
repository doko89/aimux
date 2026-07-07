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

// ─── App Model (sidebar + content) ────────────────────────────────

type appModel struct {
	cfg         *SetupConfig
	activeTab   int // 0=auth, 1=aggregator, 2=settings
	authPage    authPageModel
	aggPage     aggregatorPageModel
	settingsPage settingsPageModel
	width       int
	height      int
}

func newAppModel(cfg *SetupConfig) appModel {
	return appModel{
		cfg:         cfg,
		activeTab:   0,
		authPage:    newAuthPage(cfg),
		aggPage:     newAggregatorPage(cfg),
		settingsPage: newSettingsPage(cfg),
	}
}

func (m appModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}

	// Route to active page first
	var cmd tea.Cmd
	var handled bool
	switch m.activeTab {
	case 0:
		newModel, c := m.authPage.Update(msg)
		if newPage, ok := newModel.(authPageModel); ok {
			m.authPage = newPage
			cmd = c
			handled = true
		}
	case 1:
		newModel, c := m.aggPage.Update(msg)
		if newPage, ok := newModel.(aggregatorPageModel); ok {
			m.aggPage = newPage
			cmd = c
			handled = true
		}
	case 2:
		newModel, c := m.settingsPage.Update(msg)
		if newPage, ok := newModel.(settingsPageModel); ok {
			m.settingsPage = newPage
			cmd = c
			handled = true
		}
	}

	if !handled {
		// Check for tab switching
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "tab":
				m.activeTab = (m.activeTab + 1) % 3
				m.authPage.focus = 0
				m.aggPage.focus = 0
				m.settingsPage.focus = 0
				return m, nil
			case "shift+tab":
				m.activeTab = (m.activeTab + 2) % 3
				m.authPage.focus = 0
				m.aggPage.focus = 0
				m.settingsPage.focus = 0
				return m, nil
			case "s", "S":
				dir, _ := os.Getwd()
				if err := Save(m.cfg, dir); err != nil {
					fmt.Fprintf(os.Stderr, "Save error: %v\n", err)
				}
				return m, tea.Quit
			}
		}
	}

	return m, cmd
}

func (m appModel) View() string {
	tabs := []string{" Auth ", " Aggregator ", " Settings "}
	var sidebar string
	sidebar += "aimux setup\n\n"
	for i, t := range tabs {
		style := inactiveTabStyle
		prefix := "  "
		if i == m.activeTab {
			style = activeTabStyle
			prefix = "▸ "
		}
		sidebar += prefix + style.Render(t) + "\n"
	}
	sidebar += "\n" + helpText.Render(" tab / shift+tab") + "\n"
	sidebar += helpText.Render(" s = save & exit") + "\n"
	sidebar += helpText.Render(" q = quit")

	var content string
	switch m.activeTab {
	case 0:
		content = m.authPage.View()
	case 1:
		content = m.aggPage.View()
	case 2:
		content = m.settingsPage.View()
	}

	// Layout
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebarStyle.Render(sidebar),
		contentStyle.Render(content),
	)
}

// ─── Save helper ───────

func saveAndQuit(cfg *SetupConfig) tea.Cmd {
	return func() tea.Msg {
		dir, _ := os.Getwd()
		if err := Save(cfg, dir); err != nil {
			return saveErrMsg{err.Error()}
		}
		return quitMsg{}
	}
}

type quitMsg struct{}
type saveErrMsg struct{ s string }
