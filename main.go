package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"flight_trmnl/internal/config"
	"flight_trmnl/internal/database"
	"flight_trmnl/internal/dump1090"
	"flight_trmnl/internal/models"
	"flight_trmnl/internal/tasks"
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

	// Initialize database
	db, err := database.New(cfg.DBPath)
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Setup beast message repository
	beastRepo := db.BeastMessageRepository()

	// Setup aircraft repository
	aircraftRepo := db.AircraftRepository()
	populated, err := aircraftRepo.IsTablePopulated()
	if err != nil {
		slog.Error("Failed to check aircraft table", "error", err)
		os.Exit(1)
	}
	if !populated {
		csvPaths := []string{
			"internal/database/datasets/aircraft-database-part1.csv",
			"internal/database/datasets/aircraft-database-part2.csv",
		}
		slog.Info("Aircraft table is empty, loading from CSV files", "csv_paths", csvPaths)

		batchSize := 5000 // large batch size for efficient loading expect > 500,000 records
		if err := aircraftRepo.LoadFromMultipleCSV(csvPaths, batchSize); err != nil {
			slog.Error("Failed to load aircraft from CSV", "error", err)
			os.Exit(1)
		}
		slog.Info("Successfully loaded aircraft database from CSV")
	} else {
		slog.Info("Aircraft table is already populated")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	messageChan := make(chan *models.BeastMessage, 1000) // buffered channel for high message rate (~200/sec)
	beastClient := dump1090.NewBeastClient(cfg.BeastAddr)

	slog.Info("Starting Beast message collector", "beast_addr", cfg.BeastAddr)
	go func() {
		if err := beastClient.StreamMessages(ctx, messageChan); err != nil {
			if ctx.Err() == nil { // Only log if not cancelled
				slog.Error("Beast streamer stopped", "error", err)
			}
		}
		close(messageChan)
	}()

	// Start collector to batch and store messages in database
	collector := tasks.NewBeastCollector(beastRepo, messageChan)
	go func() {
		if err := collector.Start(ctx); err != nil && ctx.Err() == nil {
			slog.Error("Beast collector stopped", "error", err)
		}
	}()

	// Wait for interrupt signal
	<-sigChan
	slog.Info("Received interrupt signal, shutting down...")

	cancel()

	if err := beastClient.Close(); err != nil {
		slog.Error("Error closing Beast client", "error", err)
	}

	// Give collector time to flush final batch
	time.Sleep(500 * time.Millisecond)

	slog.Info("Shutdown complete")
}
