package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"bicycle/cmd"
	"bicycle/internal/config"
	"bicycle/plugin"

	"github.com/gorilla/websocket"
)

// init registers the WebSocket plugin
func init() {
	plugin.Register(NewWebSocketPlugin())
}

// WebSocketPlugin provides WebSocket server integration
type WebSocketPlugin struct {
	broker  plugin.MessageBroker
	router  *cmd.Router
	msgCh   <-chan plugin.Message
	ctx     context.Context
	server  *http.Server
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex
	upgrader websocket.Upgrader
}

// WSMessage represents a WebSocket message
type WSMessage struct {
	Type    string                 `json:"type"`    // "command", "chat", "notification"
	Payload string                 `json:"payload"` // Message content
	Data    map[string]interface{} `json:"data,omitempty"`
}

// NewWebSocketPlugin creates a new WebSocket plugin
func NewWebSocketPlugin() *WebSocketPlugin {
	return &WebSocketPlugin{
		clients: make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// TODO: Add origin checking for security
				return true
			},
		},
	}
}

// Name returns the plugin name
func (p *WebSocketPlugin) Name() string {
	return "websocket"
}

// CheckRequirements validates plugin requirements
func (p *WebSocketPlugin) CheckRequirements(ctx context.Context) error {
	checker := plugin.NewRequirementChecker("websocket")

	// Require daemon mode
	checker.AddRequired(
		"daemon_mode",
		"WebSocket requires daemon mode",
		plugin.RequireMode(plugin.ModeDaemon),
	)

	return checker.Check(ctx)
}

// Extensions returns the plugin's extensions
func (p *WebSocketPlugin) Extensions() []plugin.Extension {
	return []plugin.Extension{}
}

// Start initializes the WebSocket server
func (p *WebSocketPlugin) Start(ctx context.Context, broker plugin.MessageBroker) error {
	p.broker = broker
	p.ctx = ctx
	p.router = cmd.NewRouter()

	// Get port from config
	port := 8080
	if cfg, ok := ctx.Value("config").(*config.Config); ok {
		if portVal, ok := cfg.GetPluginSettingInt("websocket", "port"); ok {
			port = portVal
		}
	}

	// Subscribe to broker messages
	p.msgCh = broker.Subscribe("websocket", 100, "notification", "response")

	// Start broker message handler
	go p.handleBrokerMessages()

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", p.handleWebSocket)

	p.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Start server
	go func() {
		log.Printf("[WebSocket] Starting server on port %d", port)
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[WebSocket] Server error: %v", err)
		}
	}()

	log.Printf("[WebSocket] Started")
	return nil
}

// Stop shuts down the WebSocket server
func (p *WebSocketPlugin) Stop(ctx context.Context) error {
	// Close all client connections
	p.mu.Lock()
	for conn := range p.clients {
		conn.Close()
	}
	p.clients = make(map[*websocket.Conn]bool)
	p.mu.Unlock()

	// Shutdown server
	if p.server != nil {
		if err := p.server.Shutdown(ctx); err != nil {
			log.Printf("[WebSocket] Error shutting down server: %v", err)
		}
	}

	// Unsubscribe from broker
	if p.broker != nil {
		p.broker.Unsubscribe("websocket")
	}

	log.Printf("[WebSocket] Stopped")
	return nil
}

// handleWebSocket handles WebSocket connections
func (p *WebSocketPlugin) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade connection
	conn, err := p.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocket] Upgrade error: %v", err)
		return
	}

	// Register client
	p.mu.Lock()
	p.clients[conn] = true
	p.mu.Unlock()

	log.Printf("[WebSocket] Client connected from %s", r.RemoteAddr)

	// Send welcome message
	p.sendToClient(conn, WSMessage{
		Type:    "notification",
		Payload: "Connected to Bicycle daemon",
	})

	// Handle client messages
	go p.handleClientMessages(conn)
}

// handleClientMessages receives and processes messages from a WebSocket client
func (p *WebSocketPlugin) handleClientMessages(conn *websocket.Conn) {
	defer func() {
		// Unregister client
		p.mu.Lock()
		delete(p.clients, conn)
		p.mu.Unlock()
		conn.Close()
		log.Printf("[WebSocket] Client disconnected")
	}()

	for {
		var msg WSMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WebSocket] Read error: %v", err)
			}
			break
		}

		log.Printf("[WebSocket] Received: type=%s, payload=%s", msg.Type, msg.Payload)

		// Process message based on type
		switch msg.Type {
		case "command":
			p.handleCommand(conn, msg.Payload)

		case "chat":
			p.handleChat(msg.Payload)

		default:
			p.sendToClient(conn, WSMessage{
				Type:    "error",
				Payload: fmt.Sprintf("Unknown message type: %s", msg.Type),
			})
		}
	}
}

// handleCommand processes a command from WebSocket
func (p *WebSocketPlugin) handleCommand(conn *websocket.Conn, command string) {
	result, err := p.router.Route(p.ctx, command)
	if err != nil {
		p.sendToClient(conn, WSMessage{
			Type:    "error",
			Payload: err.Error(),
		})
		return
	}

	if result != nil {
		p.sendToClient(conn, WSMessage{
			Type:    "response",
			Payload: result.Output,
			Data:    map[string]interface{}{"result": result.Data},
		})

		// Broadcast if requested
		if result.Broadcast {
			p.broker.Publish(p.ctx, plugin.Message{
				Topic:   "notification",
				Payload: result.Output,
				Source:  "websocket",
			})
		}
	}
}

// handleChat processes a chat message from WebSocket
func (p *WebSocketPlugin) handleChat(text string) {
	// Publish to broker
	p.broker.Publish(p.ctx, plugin.Message{
		Topic:   "chat",
		Payload: text,
		Source:  "websocket",
	})
}

// handleBrokerMessages receives messages from the broker and broadcasts to clients
func (p *WebSocketPlugin) handleBrokerMessages() {
	for msg := range p.msgCh {
		// Convert message to WSMessage
		var text string
		if str, ok := msg.Payload.(string); ok {
			text = str
		} else {
			text = fmt.Sprintf("%v", msg.Payload)
		}

		wsMsg := WSMessage{
			Type:    msg.Topic,
			Payload: text,
		}

		// Broadcast to all clients
		p.broadcast(wsMsg)
	}
}

// sendToClient sends a message to a specific client
func (p *WebSocketPlugin) sendToClient(conn *websocket.Conn, msg WSMessage) {
	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("[WebSocket] Write error: %v", err)
	}
}

// broadcast sends a message to all connected clients
func (p *WebSocketPlugin) broadcast(msg WSMessage) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	data, _ := json.Marshal(msg)
	log.Printf("[WebSocket] Broadcasting: %s", string(data))

	for conn := range p.clients {
		if err := conn.WriteJSON(msg); err != nil {
			log.Printf("[WebSocket] Broadcast error: %v", err)
		}
	}
}
