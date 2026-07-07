package setup

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FAFAFA")).
				Background(lipgloss.Color("#7D56F4")).
				Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFB86C"))

	normalStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#F8F8F2"))

	helpStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#6272A4"))
)

// Run starts the interactive setup TUI.
func Run() {
	cfg := LoadFromExisting()
	p := tea.NewProgram(newMainMenu(cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// main menu
type mainMenuModel struct {
	cfg     *SetupConfig
	cursor  int
	choices []string
	done    bool
	quit    bool
}

func newMainMenu(cfg *SetupConfig) mainMenuModel {
	return mainMenuModel{
		cfg: cfg,
		choices: []string{
			"Gateway Settings",
			"Providers",
			"Model Aggregations",
			"Routing",
			"Circuit Breaker",
			"Rate Limiting",
			"Auth (API Keys)",
			"Login (ChatGPT)",
			"\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500",
			"Save & Exit",
		},
	}
}

func (m mainMenuModel) Init() tea.Cmd { return nil }

func (m mainMenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quit = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			choice := m.choices[m.cursor]
			switch choice {
			case "Save & Exit":
				dir, _ := os.Getwd()
				if err := Save(m.cfg, dir); err != nil {
					fmt.Fprintf(os.Stderr, "Save error: %v\n", err)
				} else {
					fmt.Println("\n\u2713 Config saved to .env + aggregation.yaml")
				}
				m.done = true
				return m, tea.Quit
			case "Gateway Settings":
				return newGatewayModel(m.cfg, &m), nil
			case "Providers":
				return newProviderListModel(m.cfg, &m), nil
			case "Model Aggregations":
				return newAggregationListModel(m.cfg, &m), nil
			case "Routing":
				return newRoutingModel(m.cfg, &m), nil
			case "Circuit Breaker":
				return newCircuitBreakerModel(m.cfg, &m), nil
			case "Rate Limiting":
				return newRateLimitModel(m.cfg, &m), nil
			case "Auth (API Keys)":
				return newAuthModel(m.cfg, &m), nil
			case "Login (ChatGPT)":
				return newLoginModel(m.cfg, &m), nil
			}
		}
	}
	return m, nil
}

func (m mainMenuModel) View() string {
	if m.quit || m.done {
		return ""
	}

	s := titleStyle.Render("  aimux setup  ") + "\n\n"
	s += helpStyle.Render("  Select a section to configure:\n\n")

	for i, choice := range m.choices {
		cursor := "  "
		if m.cursor == i {
			cursor = selectedStyle.Render("\u25b6 ")
			choice = selectedStyle.Render(choice)
		} else {
			choice = normalStyle.Render(choice)
		}
		s += "  " + cursor + choice + "\n"
	}

	s += "\n" + helpStyle.Render("  j/k or arrows to navigate | enter to select | q to quit")
	return s
}

// backToMenu is a helper that returns a Cmd to go back to main menu.
func backToMenu() tea.Msg {
	return backMsg{}
}

type backMsg struct{}

func isBackMsg(msg tea.Msg) bool {
	_, ok := msg.(backMsg)
	return ok
}
