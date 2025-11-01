package config

import (
	"fmt"
	"os"

	"bicycle/plugin"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	// Daemon configuration
	Daemon DaemonConfig `yaml:"daemon"`

	// Plugin configurations
	Plugins map[string]PluginConfig `yaml:"plugins"`

	// Mode specifies the execution mode
	Mode plugin.Mode `yaml:"mode"`
}

// DaemonConfig contains daemon-specific configuration
type DaemonConfig struct {
	// LogLevel specifies the logging level (debug, info, warn, error)
	LogLevel string `yaml:"log_level"`

	// BrokerBufferSize is the default buffer size for message broker subscriptions
	BrokerBufferSize int `yaml:"broker_buffer_size"`

	// PublishTimeout is the timeout for publishing messages (in seconds)
	PublishTimeout int `yaml:"publish_timeout"`
}

// PluginConfig contains configuration for a specific plugin
type PluginConfig struct {
	// Enabled indicates if the plugin should be loaded
	Enabled bool `yaml:"enabled"`

	// Settings contains plugin-specific settings
	Settings map[string]interface{} `yaml:"settings"`
}

// Load loads configuration from a YAML file
func Load(path string) (*Config, error) {
	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults
	cfg.applyDefaults()

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// LoadOrDefault loads configuration from a file or returns default config
func LoadOrDefault(path string) (*Config, error) {
	if path == "" || !fileExists(path) {
		return DefaultConfig(), nil
	}
	return Load(path)
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	cfg := &Config{
		Daemon: DaemonConfig{
			LogLevel:         "info",
			BrokerBufferSize: 100,
			PublishTimeout:   5,
		},
		Plugins: make(map[string]PluginConfig),
		Mode:    plugin.ModeDaemon,
	}
	return cfg
}

// applyDefaults applies default values to missing configuration
func (c *Config) applyDefaults() {
	// Daemon defaults
	if c.Daemon.LogLevel == "" {
		c.Daemon.LogLevel = "info"
	}
	if c.Daemon.BrokerBufferSize == 0 {
		c.Daemon.BrokerBufferSize = 100
	}
	if c.Daemon.PublishTimeout == 0 {
		c.Daemon.PublishTimeout = 5
	}

	// Mode defaults
	if c.Mode == "" {
		c.Mode = plugin.ModeDaemon
	}

	// Ensure plugins map exists
	if c.Plugins == nil {
		c.Plugins = make(map[string]PluginConfig)
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate mode
	if c.Mode != plugin.ModeDaemon && c.Mode != plugin.ModeInteractive {
		return fmt.Errorf("invalid mode: %s (must be 'daemon' or 'interactive')", c.Mode)
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.Daemon.LogLevel] {
		return fmt.Errorf("invalid log level: %s", c.Daemon.LogLevel)
	}

	// Validate buffer size
	if c.Daemon.BrokerBufferSize < 1 {
		return fmt.Errorf("broker buffer size must be at least 1")
	}

	// Validate publish timeout
	if c.Daemon.PublishTimeout < 1 {
		return fmt.Errorf("publish timeout must be at least 1 second")
	}

	return nil
}

// GetPluginConfig returns configuration for a specific plugin
func (c *Config) GetPluginConfig(name string) (PluginConfig, bool) {
	cfg, exists := c.Plugins[name]
	return cfg, exists
}

// IsPluginEnabled checks if a plugin is enabled in the configuration
func (c *Config) IsPluginEnabled(name string) bool {
	cfg, exists := c.Plugins[name]
	if !exists {
		// If not specified in config, assume enabled
		return true
	}
	return cfg.Enabled
}

// GetPluginSetting retrieves a specific setting for a plugin
func (c *Config) GetPluginSetting(pluginName, settingName string) (interface{}, bool) {
	cfg, exists := c.Plugins[pluginName]
	if !exists || cfg.Settings == nil {
		return nil, false
	}

	val, exists := cfg.Settings[settingName]
	return val, exists
}

// GetPluginSettingString retrieves a string setting for a plugin
func (c *Config) GetPluginSettingString(pluginName, settingName string) (string, bool) {
	val, exists := c.GetPluginSetting(pluginName, settingName)
	if !exists {
		return "", false
	}

	str, ok := val.(string)
	return str, ok
}

// GetPluginSettingInt retrieves an int setting for a plugin
func (c *Config) GetPluginSettingInt(pluginName, settingName string) (int, bool) {
	val, exists := c.GetPluginSetting(pluginName, settingName)
	if !exists {
		return 0, false
	}

	// YAML unmarshals integers as int
	if i, ok := val.(int); ok {
		return i, true
	}

	return 0, false
}

// GetPluginSettingBool retrieves a bool setting for a plugin
func (c *Config) GetPluginSettingBool(pluginName, settingName string) (bool, bool) {
	val, exists := c.GetPluginSetting(pluginName, settingName)
	if !exists {
		return false, false
	}

	b, ok := val.(bool)
	return b, ok
}

// Save writes the configuration to a YAML file
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
