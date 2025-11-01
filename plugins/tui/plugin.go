package tui

import (
	"context"
	"fmt"
	"log"
	"strings"

	"bicycle/cmd"
	"bicycle/plugin"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// init registers the TUI plugin
func init() {
	plugin.Register(NewTUIPlugin())
}

// TUIPlugin provides a terminal user interface
type TUIPlugin struct {
	program *tea.Program
	model   *model
	broker  plugin.MessageBroker
	msgCh   <-chan plugin.Message
	ctx     context.Context
}

// NewTUIPlugin creates a new TUI plugin
func NewTUIPlugin() *TUIPlugin {
	return &TUIPlugin{}
}

// Name returns the plugin name
func (p *TUIPlugin) Name() string {
	return "tui"
}

// CheckRequirements validates plugin requirements
func (p *TUIPlugin) CheckRequirements(ctx context.Context) error {
	checker := plugin.NewRequirementChecker("tui")

	// Require interactive mode
	checker.AddRequired(
		"interactive_mode",
		"TUI requires interactive mode",
		plugin.RequireMode(plugin.ModeInteractive),
	)

	return checker.Check(ctx)
}

// Extensions returns the plugin's extensions
func (p *TUIPlugin) Extensions() []plugin.Extension {
	return []plugin.Extension{}
}

// Start initializes the TUI
func (p *TUIPlugin) Start(ctx context.Context, broker plugin.MessageBroker) error {
	p.broker = broker
	p.ctx = ctx

	// Subscribe to messages
	p.msgCh = broker.Subscribe("tui", 100, "notification", "chat", "response")

	// Create model
	p.model = newModel(ctx, broker)

	// Start bubbletea program
	p.program = tea.NewProgram(p.model, tea.WithAltScreen())

	// Handle incoming messages in background
	go p.handleMessages()

	// Run TUI (this blocks)
	go func() {
		if _, err := p.program.Run(); err != nil {
			log.Printf("[TUI] Error running program: %v", err)
		}
	}()

	log.Printf("[TUI] Started")
	return nil
}

// Stop shuts down the TUI
func (p *TUIPlugin) Stop(ctx context.Context) error {
	if p.program != nil {
		p.program.Quit()
	}

	if p.broker != nil {
		p.broker.Unsubscribe("tui")
	}

	log.Printf("[TUI] Stopped")
	return nil
}

// handleMessages receives messages from the broker and updates the TUI
func (p *TUIPlugin) handleMessages() {
	for {
		select {
		case msg, ok := <-p.msgCh:
			if !ok {
				return
			}

			// Convert message to string
			var text string
			if str, ok := msg.Payload.(string); ok {
				text = str
			} else {
				text = fmt.Sprintf("%v", msg.Payload)
			}

			// Send to bubbletea model
			if p.program != nil {
				p.program.Send(incomingMessageMsg{
					source: msg.Source,
					text:   text,
				})
			}

		case <-p.ctx.Done():
			return
		}
	}
}

// model represents the bubbletea model
type model struct {
	ctx      context.Context
	broker   plugin.MessageBroker
	router   *cmd.Router
	messages []message
	input    string
	width    int
	height   int
}

// message represents a chat message
type message struct {
	source string
	text   string
}

// incomingMessageMsg is a bubbletea message for incoming broker messages
type incomingMessageMsg struct {
	source string
	text   string
}

// newModel creates a new bubbletea model
func newModel(ctx context.Context, broker plugin.MessageBroker) *model {
	return &model{
		ctx:      ctx,
		broker:   broker,
		router:   cmd.NewRouter(),
		messages: []message{{source: "system", text: "Welcome to Bicycle! Type /help for commands."}},
		input:    "",
	}
}

// Init initializes the model
func (m *model) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyEnter:
			if m.input != "" {
				// Add user message
				m.messages = append(m.messages, message{
					source: "you",
					text:   m.input,
				})

				// Process command
				input := m.input
				m.input = ""

				go m.processCommand(input)
			}

		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}

		default:
			m.input += msg.String()
		}

	case incomingMessageMsg:
		// Add message from broker
		m.messages = append(m.messages, message{
			source: msg.source,
			text:   msg.text,
		})

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

// processCommand processes a user command
func (m *model) processCommand(input string) {
	// Check if it's a command
	if !strings.HasPrefix(input, "/") {
		// Regular message - publish to broker
		m.broker.Publish(m.ctx, plugin.Message{
			Topic:   "chat",
			Payload: input,
			Source:  "tui",
		})
		return
	}

	// Execute command
	result, err := m.router.Route(m.ctx, input)
	if err != nil {
		m.addMessage("error", fmt.Sprintf("Error: %v", err))
		return
	}

	if result != nil && result.Output != "" {
		m.addMessage("system", result.Output)

		// Broadcast if requested
		if result.Broadcast {
			m.broker.Publish(m.ctx, plugin.Message{
				Topic:   "notification",
				Payload: result.Output,
				Source:  "tui",
			})
		}
	}
}

// addMessage adds a message to the chat
func (m *model) addMessage(source, text string) {
	// Send via program to ensure thread-safety
	if p, ok := m.ctx.Value("program").(*tea.Program); ok {
		p.Send(incomingMessageMsg{source: source, text: text})
	}
}

// View renders the UI
func (m *model) View() string {
	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Padding(0, 1)

	messageStyle := lipgloss.NewStyle().
		Padding(0, 2)

	userStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Bold(true)

	systemStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true)

	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true).
		Padding(0, 1)

	// Build UI
	var s strings.Builder

	// Title
	s.WriteString(titleStyle.Render("Bicycle Daemon"))
	s.WriteString("\n\n")

	// Messages (show last N messages that fit)
	availableHeight := m.height - 6 // Reserve space for title and input
	if availableHeight < 1 {
		availableHeight = 10
	}

	start := 0
	if len(m.messages) > availableHeight {
		start = len(m.messages) - availableHeight
	}

	for _, msg := range m.messages[start:] {
		var prefix string
		var style lipgloss.Style

		switch msg.source {
		case "you":
			prefix = "You: "
			style = userStyle
		case "system":
			prefix = "System: "
			style = systemStyle
		case "error":
			prefix = "Error: "
			style = errorStyle
		default:
			prefix = fmt.Sprintf("[%s]: ", msg.source)
			style = messageStyle
		}

		s.WriteString(messageStyle.Render(style.Render(prefix) + msg.text))
		s.WriteString("\n")
	}

	// Input
	s.WriteString("\n")
	s.WriteString(inputStyle.Render("> " + m.input))

	// Help text
	s.WriteString("\n\n")
	s.WriteString(systemStyle.Render("Press Ctrl+C or Esc to quit | Type /help for commands"))

	return s.String()
}
