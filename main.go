package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"flight_trmnl/internal/daemon"
)

func main() {
	// Configuration
	cfg := daemon.Config{
		DBPath:       "adsb_data.db",
		BeastAddr:    "localhost:30005", // Beast format address
		BatchSize:    100,                // Batch 100 messages before writing
		BatchTimeout: 5,                  // Flush batch after 5 seconds
	}

	// Allow override via environment variables
	if envDBPath := os.Getenv("ADSB_DB_PATH"); envDBPath != "" {
		cfg.DBPath = envDBPath
	}
	if envBeastAddr := os.Getenv("BEAST_ADDR"); envBeastAddr != "" {
		cfg.BeastAddr = envBeastAddr
	}
	if envBatchSize := os.Getenv("BATCH_SIZE"); envBatchSize != "" {
		if batchSize, err := strconv.Atoi(envBatchSize); err == nil {
			cfg.BatchSize = batchSize
		}
	}
	if envBatchTimeout := os.Getenv("BATCH_TIMEOUT"); envBatchTimeout != "" {
		if batchTimeout, err := strconv.Atoi(envBatchTimeout); err == nil {
			cfg.BatchTimeout = batchTimeout
		}
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create and start the daemon
	d, err := daemon.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create daemon: %v", err)
	}

	if err := d.Start(); err != nil {
		log.Fatalf("Failed to start daemon: %v", err)
	}

	// Wait for interrupt signal
	<-sigChan
	fmt.Println("\nReceived interrupt signal, shutting down...")

	// Stop the daemon gracefully
	if err := d.Stop(); err != nil {
		log.Fatalf("Failed to stop daemon: %v", err)
	}
}
