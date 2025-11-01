package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"bicycle/daemon"
	"bicycle/internal/config"
	"bicycle/plugin"

	// Import all plugins (triggers init registration)
	_ "bicycle/cmd"
	_ "bicycle/plugins/executor/llm"
	_ "bicycle/plugins/rest"
	_ "bicycle/plugins/state/memory"
	_ "bicycle/plugins/telegram"
	_ "bicycle/plugins/tui"
	_ "bicycle/plugins/websocket"
)

var (
	version = "0.1.0"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	mode := flag.String("mode", "", "Execution mode (daemon or interactive)")
	showVersion := flag.Bool("version", false, "Show version information")
	listPlugins := flag.Bool("list-plugins", false, "List registered plugins")

	flag.Parse()

	// Show version
	if *showVersion {
		fmt.Printf("Bicycle v%s\n", version)
		fmt.Println("A modular Go daemon service with plugin architecture")
		return
	}

	// List plugins
	if *listPlugins {
		registry := plugin.GetRegistry()
		plugins := registry.All()

		fmt.Printf("Registered plugins (%d):\n\n", len(plugins))
		for i, p := range plugins {
			fmt.Printf("%d. %s\n", i+1, p.Name())

			extensions := p.Extensions()
			if len(extensions) > 0 {
				fmt.Printf("   Extensions:\n")
				for _, ext := range extensions {
					fmt.Printf("     - %s:%s\n", ext.Type(), ext.Name())
				}
			}
		}
		return
	}

	// Load configuration
	cfg, err := config.LoadOrDefault(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Override mode if specified via CLI
	if *mode != "" {
		cfg.Mode = plugin.Mode(*mode)
		if err := cfg.Validate(); err != nil {
			log.Fatalf("Invalid mode: %v", err)
		}
	}

	// Print startup banner
	printBanner(cfg)

	// Create daemon
	d := daemon.New(cfg)

	// Load plugins from registry
	registry := plugin.GetRegistry()
	allPlugins := registry.All()

	log.Printf("Found %d registered plugin(s)", len(allPlugins))

	// Add plugins to daemon
	for _, p := range allPlugins {
		if err := d.AddPlugin(p); err != nil {
			log.Printf("Failed to add plugin %s: %v", p.Name(), err)
		}
	}

	// Start daemon
	if err := d.Start(); err != nil {
		log.Fatalf("Failed to start daemon: %v", err)
	}

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal
	log.Println("Daemon running. Press Ctrl+C to stop.")
	<-sigCh

	log.Println("Shutdown signal received, stopping...")

	// Stop daemon
	if err := d.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Println("Daemon stopped")
}

// printBanner prints the startup banner
func printBanner(cfg *config.Config) {
	fmt.Println("╔════════════════════════════════════════════╗")
	fmt.Println("║            Bicycle Daemon v" + version + "           ║")
	fmt.Println("╚════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("Mode: %s\n", cfg.Mode)
	fmt.Printf("Config: loaded\n")
	fmt.Println()
}
