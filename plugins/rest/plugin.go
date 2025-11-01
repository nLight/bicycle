package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"bicycle/cmd"
	"bicycle/internal/config"
	"bicycle/plugin"
)

// init registers the REST API plugin
func init() {
	plugin.Register(NewRESTPlugin())
}

// RESTPlugin provides REST API integration
type RESTPlugin struct {
	broker plugin.MessageBroker
	router *cmd.Router
	ctx    context.Context
	server *http.Server
	authToken string
}

// CommandRequest represents a command request
type CommandRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// CommandResponse represents a command response
type CommandResponse struct {
	Success bool        `json:"success"`
	Output  string      `json:"output,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// StatusResponse represents a status response
type StatusResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// NewRESTPlugin creates a new REST API plugin
func NewRESTPlugin() *RESTPlugin {
	return &RESTPlugin{}
}

// Name returns the plugin name
func (p *RESTPlugin) Name() string {
	return "rest"
}

// CheckRequirements validates plugin requirements
func (p *RESTPlugin) CheckRequirements(ctx context.Context) error {
	checker := plugin.NewRequirementChecker("rest")

	// Require daemon mode
	checker.AddRequired(
		"daemon_mode",
		"REST API requires daemon mode",
		plugin.RequireMode(plugin.ModeDaemon),
	)

	return checker.Check(ctx)
}

// Extensions returns the plugin's extensions
func (p *RESTPlugin) Extensions() []plugin.Extension {
	return []plugin.Extension{}
}

// Start initializes the REST API server
func (p *RESTPlugin) Start(ctx context.Context, broker plugin.MessageBroker) error {
	p.broker = broker
	p.ctx = ctx
	p.router = cmd.NewRouter()

	// Get configuration
	port := 8081
	host := "0.0.0.0"

	if cfg, ok := ctx.Value("config").(*config.Config); ok {
		if portVal, ok := cfg.GetPluginSettingInt("rest", "port"); ok {
			port = portVal
		}
		if hostVal, ok := cfg.GetPluginSettingString("rest", "host"); ok {
			host = hostVal
		}
		if token, ok := cfg.GetPluginSettingString("rest", "auth_token"); ok {
			p.authToken = token
		}
	}

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/api/command", p.authMiddleware(p.handleCommand))
	mux.HandleFunc("/api/status", p.authMiddleware(p.handleStatus))
	mux.HandleFunc("/api/health", p.handleHealth)

	p.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", host, port),
		Handler: mux,
	}

	// Start server
	go func() {
		log.Printf("[REST] Starting server on %s:%d", host, port)
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[REST] Server error: %v", err)
		}
	}()

	log.Printf("[REST] Started")
	return nil
}

// Stop shuts down the REST API server
func (p *RESTPlugin) Stop(ctx context.Context) error {
	if p.server != nil {
		if err := p.server.Shutdown(ctx); err != nil {
			log.Printf("[REST] Error shutting down server: %v", err)
		}
	}

	log.Printf("[REST] Stopped")
	return nil
}

// authMiddleware adds optional authentication
func (p *RESTPlugin) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If auth token is configured, check it
		if p.authToken != "" {
			token := r.Header.Get("Authorization")
			expectedToken := "Bearer " + p.authToken

			if token != expectedToken {
				p.sendError(w, http.StatusUnauthorized, "Unauthorized")
				return
			}
		}

		next(w, r)
	}
}

// handleCommand processes command requests
func (p *RESTPlugin) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		p.sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Parse request
	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		p.sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Printf("[REST] Command request: %s %v", req.Command, req.Args)

	// Execute command
	result, err := p.router.Route(p.ctx, req.Command)
	if err != nil {
		p.sendJSON(w, CommandResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Send response
	response := CommandResponse{
		Success: true,
	}

	if result != nil {
		response.Output = result.Output
		response.Data = result.Data

		// Broadcast if requested
		if result.Broadcast {
			p.broker.Publish(p.ctx, plugin.Message{
				Topic:   "notification",
				Payload: result.Output,
				Source:  "rest",
			})
		}
	}

	p.sendJSON(w, response)
}

// handleStatus returns daemon status
func (p *RESTPlugin) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		p.sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Get status from daemon
	var statusText string
	if daemon, ok := p.ctx.Value("daemon").(interface {
		GetStatus(context.Context) string
	}); ok {
		statusText = daemon.GetStatus(p.ctx)
	} else {
		statusText = "Status not available"
	}

	p.sendJSON(w, StatusResponse{
		Status:  "ok",
		Message: statusText,
	})
}

// handleHealth returns health check
func (p *RESTPlugin) handleHealth(w http.ResponseWriter, r *http.Request) {
	p.sendJSON(w, map[string]string{
		"status": "healthy",
	})
}

// sendJSON sends a JSON response
func (p *RESTPlugin) sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[REST] Error encoding response: %v", err)
	}
}

// sendError sends an error response
func (p *RESTPlugin) sendError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
