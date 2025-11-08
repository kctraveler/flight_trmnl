package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"flight_trmnl/internal/config"
	"flight_trmnl/internal/daemon"
)

func initLogger(cfg *config.Config) {
	var logLevel slog.Level
	switch cfg.Log.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	var handler slog.Handler
	if cfg.Log.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}

func main() {
	configPath := flag.String("config", "", "Path to config file (YAML)")
	flag.Parse()

	if *configPath != "" {
		os.Setenv("FLIGHT_TRMNL_CONFIG_PATH", *configPath)
	}

	cfg, err := config.Load()
	if err != nil {
		// Use basic logging for config errors since logger isn't initialized yet
		// Initialize a basic logger just for this error
		basicLogger := slog.New(slog.NewTextHandler(os.Stderr, nil))
		basicLogger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	initLogger(cfg)

	daemonCfg := daemon.Config{
		DBPath:       cfg.DBPath,
		BeastAddr:    cfg.BeastAddr,
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.BatchTimeout,
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create and start the daemon
	d, err := daemon.New(daemonCfg)
	if err != nil {
		slog.Error("Failed to create daemon", "error", err)
		os.Exit(1)
	}

	if err := d.Start(); err != nil {
		slog.Error("Failed to start daemon", "error", err)
		os.Exit(1)
	}

	// Wait for interrupt signal
	<-sigChan
	slog.Info("Received interrupt signal, shutting down...")

	// Stop the daemon gracefully
	if err := d.Stop(); err != nil {
		slog.Error("Failed to stop daemon", "error", err)
		os.Exit(1)
	}
}
