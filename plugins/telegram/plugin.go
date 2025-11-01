package telegram

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"bicycle/cmd"
	"bicycle/internal/config"
	"bicycle/plugin"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// init registers the Telegram plugin
func init() {
	plugin.Register(NewTelegramPlugin())
}

// TelegramPlugin provides Telegram bot integration
type TelegramPlugin struct {
	bot    *tgbotapi.BotAPI
	broker plugin.MessageBroker
	router *cmd.Router
	msgCh  <-chan plugin.Message
	ctx    context.Context
	stopCh chan struct{}
	chatID int64 // Active chat ID for sending messages
}

// NewTelegramPlugin creates a new Telegram plugin
func NewTelegramPlugin() *TelegramPlugin {
	return &TelegramPlugin{
		stopCh: make(chan struct{}),
	}
}

// Name returns the plugin name
func (p *TelegramPlugin) Name() string {
	return "telegram"
}

// CheckRequirements validates plugin requirements
func (p *TelegramPlugin) CheckRequirements(ctx context.Context) error {
	checker := plugin.NewRequirementChecker("telegram")

	// Get token from config or environment
	token := p.getToken(ctx)

	// Require token
	checker.AddRequired(
		"telegram_token",
		"Telegram bot token required",
		func(ctx context.Context) error {
			if token == "" {
				return fmt.Errorf("TELEGRAM_TOKEN not set in config or environment")
			}
			return nil
		},
	)

	// Require daemon mode
	checker.AddRequired(
		"daemon_mode",
		"Telegram requires daemon mode",
		plugin.RequireMode(plugin.ModeDaemon),
	)

	return checker.Check(ctx)
}

// getToken retrieves the Telegram token from config or environment
func (p *TelegramPlugin) getToken(ctx context.Context) string {
	// Try config first
	if cfg, ok := ctx.Value("config").(*config.Config); ok {
		if token, ok := cfg.GetPluginSettingString("telegram", "token"); ok && token != "" {
			return token
		}
	}

	// Fallback to environment variable
	return os.Getenv("TELEGRAM_TOKEN")
}

// Extensions returns the plugin's extensions
func (p *TelegramPlugin) Extensions() []plugin.Extension {
	return []plugin.Extension{}
}

// Start initializes the Telegram bot
func (p *TelegramPlugin) Start(ctx context.Context, broker plugin.MessageBroker) error {
	p.broker = broker
	p.ctx = ctx
	p.router = cmd.NewRouter()

	// Get token
	token := p.getToken(ctx)

	// Create bot
	var err error
	p.bot, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		return fmt.Errorf("failed to create bot: %w", err)
	}

	log.Printf("[Telegram] Authorized on account %s", p.bot.Self.UserName)

	// Subscribe to broker messages
	p.msgCh = broker.Subscribe("telegram", 100, "notification", "response")

	// Start message handlers
	go p.handleBrokerMessages()
	go p.handleTelegramUpdates()

	log.Printf("[Telegram] Started")
	return nil
}

// Stop shuts down the Telegram bot
func (p *TelegramPlugin) Stop(ctx context.Context) error {
	close(p.stopCh)

	if p.bot != nil {
		p.bot.StopReceivingUpdates()
	}

	if p.broker != nil {
		p.broker.Unsubscribe("telegram")
	}

	log.Printf("[Telegram] Stopped")
	return nil
}

// handleBrokerMessages receives messages from the broker and sends to Telegram
func (p *TelegramPlugin) handleBrokerMessages() {
	for {
		select {
		case msg, ok := <-p.msgCh:
			if !ok {
				return
			}

			// Only send if we have an active chat
			if p.chatID == 0 {
				continue
			}

			// Convert message to string
			var text string
			if str, ok := msg.Payload.(string); ok {
				text = str
			} else {
				text = fmt.Sprintf("%v", msg.Payload)
			}

			// Send to Telegram
			p.sendMessage(p.chatID, text)

		case <-p.stopCh:
			return
		}
	}
}

// handleTelegramUpdates receives updates from Telegram
func (p *TelegramPlugin) handleTelegramUpdates() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := p.bot.GetUpdatesChan(u)

	for {
		select {
		case update := <-updates:
			if update.Message == nil {
				continue
			}

			// Set active chat ID
			p.chatID = update.Message.Chat.ID

			// Log message
			log.Printf("[Telegram] [%s] %s", update.Message.From.UserName, update.Message.Text)

			// Process message
			p.processMessage(update.Message)

		case <-p.stopCh:
			return
		}
	}
}

// processMessage processes a Telegram message
func (p *TelegramPlugin) processMessage(message *tgbotapi.Message) {
	text := message.Text

	// Check if it's a command
	if strings.HasPrefix(text, "/") {
		// Execute command
		result, err := p.router.Route(p.ctx, text)
		if err != nil {
			p.sendMessage(message.Chat.ID, fmt.Sprintf("Error: %v", err))
			return
		}

		if result != nil && result.Output != "" {
			p.sendMessage(message.Chat.ID, result.Output)

			// Broadcast if requested
			if result.Broadcast {
				p.broker.Publish(p.ctx, plugin.Message{
					Topic:   "notification",
					Payload: result.Output,
					Source:  "telegram",
				})
			}
		}
	} else {
		// Regular message - publish to broker
		p.broker.Publish(p.ctx, plugin.Message{
			Topic:   "chat",
			Payload: text,
			Source:  "telegram",
			Metadata: map[string]interface{}{
				"user_id":   message.From.ID,
				"username":  message.From.UserName,
				"chat_id":   message.Chat.ID,
			},
		})

		// Echo confirmation
		p.sendMessage(message.Chat.ID, "Message received")
	}
}

// sendMessage sends a message to a Telegram chat
func (p *TelegramPlugin) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := p.bot.Send(msg); err != nil {
		log.Printf("[Telegram] Error sending message: %v", err)
	}
}
