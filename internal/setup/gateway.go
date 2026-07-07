package setup

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

type gatewayModel struct {
	cfg      *SetupConfig
	back     *mainMenuModel
	host     textinput.Model
	port     textinput.Model
	debug    bool
	cursor   int
	fields   []string
	saved    bool
}

func newGatewayModel(cfg *SetupConfig, back *mainMenuModel) gatewayModel {
	h := textinput.New()
	h.Placeholder = "0.0.0.0"
	h.SetValue(cfg.Gateway.Host)
	h.Focus()

	p := textinput.New()
	p.Placeholder = "8080"
	p.SetValue(strconv.Itoa(cfg.Gateway.Port))

	return gatewayModel{
		cfg:    cfg,
		back:   back,
		host:   h,
		port:   p,
		debug:  cfg.Gateway.Debug,
		fields: []string{"Host", "Port", "Debug"},
	}
}

func (m gatewayModel) Init() tea.Cmd { return textinput.Blink }

func (m gatewayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m.back, nil
		case "tab", "down":
			if m.cursor < len(m.fields)-1 {
				m.cursor++
				if m.cursor == 0 { m.host.Focus() } else if m.cursor == 1 { m.port.Focus() }
			}
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			// Save values
			m.cfg.Gateway.Host = m.host.Value()
			if port, err := strconv.Atoi(m.port.Value()); err == nil {
				m.cfg.Gateway.Port = port
			}
			m.cfg.Gateway.Debug = m.debug
			m.saved = true
			return m, tea.Batch(func() tea.Msg { return backMsg{} })
		case " ":
			if m.cursor == 2 { m.debug = !m.debug }
		}
	}
	var cmd tea.Cmd
	if m.cursor == 0 {
		m.host, cmd = m.host.Update(msg)
	} else if m.cursor == 1 {
		m.port, cmd = m.port.Update(msg)
	}
	return m, cmd
}

func (m gatewayModel) View() string {
	s := "\n  \033[1mGateway Settings\033[0m\n\n"
	s += "  Host:     " + m.host.View() + "\n"
	s += "  Port:     " + m.port.View() + "\n"
	debugStr := "off"
	if m.debug { debugStr = "on" }
	s += fmt.Sprintf("  Debug:    %s (space to toggle)\n", debugStr)
	s += "\n  enter: save | esc: back"
	return s
}
