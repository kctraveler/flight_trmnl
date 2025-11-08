package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all configuration for the daemon
type Config struct {
	BeastAddr    string
	DBPath       string
	BatchSize    int
	BatchTimeout int
	Log          LogConfig
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string
	Format string
}

// Load loads configuration from config file and environment variables
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("beast_addr", "localhost:30005")
	v.SetDefault("db_path", "adsb_data.db")
	v.SetDefault("batch_size", 100)
	v.SetDefault("batch_timeout", 5)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "text")

	// Set config file name and type
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	// Set config file search paths
	v.AddConfigPath("/etc/flight_trmnl")
	v.AddConfigPath(".")

	// Check for config file path from environment variable
	if configPath := os.Getenv("FLIGHT_TRMNL_CONFIG_PATH"); configPath != "" {
		v.SetConfigFile(configPath)
	}

	// Check for config file path from command line flag
	// This will be handled in main.go, but we can also check here
	// For now, we'll rely on the flag being set before Load() is called

	// Read config file (if it exists)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Config file was found but another error occurred
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is OK - we'll use defaults + env vars
		// Don't log here since logger isn't initialized yet
	} else {
		// Config file was loaded successfully
		// Don't log here since logger isn't initialized yet
		_ = v.ConfigFileUsed() // Store for potential future use
	}

	// Set environment variable prefix
	v.SetEnvPrefix("FLIGHT_TRMNL")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Build config struct
	cfg := &Config{
		BeastAddr:    v.GetString("beast_addr"),
		DBPath:       v.GetString("db_path"),
		BatchSize:    v.GetInt("batch_size"),
		BatchTimeout: v.GetInt("batch_timeout"),
		Log: LogConfig{
			Level:  v.GetString("log.level"),
			Format: v.GetString("log.format"),
		},
	}

	// Validate configuration
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// validate validates the configuration values
func validate(cfg *Config) error {
	if cfg.BeastAddr == "" {
		return fmt.Errorf("beast_addr is required")
	}

	if cfg.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be greater than 0")
	}

	if cfg.BatchTimeout <= 0 {
		return fmt.Errorf("batch_timeout must be greater than 0")
	}

	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[strings.ToLower(cfg.Log.Level)] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", cfg.Log.Level)
	}

	validLogFormats := map[string]bool{
		"text": true,
		"json": true,
	}
	if !validLogFormats[strings.ToLower(cfg.Log.Format)] {
		return fmt.Errorf("invalid log format: %s (must be text or json)", cfg.Log.Format)
	}

	return nil
}

