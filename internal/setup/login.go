package setup

import (
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

type loginModel struct {
	cfg   *SetupConfig
	back  *mainMenuModel
	done  bool
	err   string
}

func newLoginModel(cfg *SetupConfig, back *mainMenuModel) loginModel {
	return loginModel{cfg: cfg, back: back}
}

func (m loginModel) Init() tea.Cmd {
	return func() tea.Msg {
		// Run aimux login chatgpt inline
		cmd := exec.Command(os.Args[0], "login", "chatgpt")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		err := cmd.Run()
		if err != nil {
			return loginDoneMsg{err: err.Error()}
		}
		return loginDoneMsg{}
	}
}

func (m loginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loginDoneMsg:
		m.done = true
		if msg.err != "" {
			m.err = msg.err
		}
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "esc" {
			return m.back, nil
		}
	}
	return m, nil
}

func (m loginModel) View() string {
	if m.done {
		if m.err != "" {
			return "\n  Login failed: " + m.err + "\n  Press any key to go back..."
		}
		return "\n  Login successful! Press any key to go back..."
	}
	return "\n  Opening ChatGPT login...\n  Follow the instructions in your browser.\n"
}

type loginDoneMsg struct {
	err string
}
