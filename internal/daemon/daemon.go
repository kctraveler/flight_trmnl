package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"flight_trmnl/internal/database"
	"flight_trmnl/internal/dump1090"
	"flight_trmnl/internal/models"
	"flight_trmnl/internal/scheduler"
	"flight_trmnl/internal/tasks"
)

// Daemon represents the main daemon structure
type Daemon struct {
	ctx         context.Context
	cancel      context.CancelFunc
	scheduler   *scheduler.Scheduler
	database    database.Repository
	beastClient *dump1090.BeastClient
	messageChan chan *models.BeastMessage
	done        chan struct{}
}

// Config holds daemon configuration
type Config struct {
	DBPath       string // Path to SQLite database
	BeastAddr    string // Beast format address (e.g., "localhost:30005")
	BatchSize    int    // Number of messages to batch before writing
	BatchTimeout int    // seconds - flush batch after this time even if not full
}

// New creates a new daemon instance
func New(cfg Config) (*Daemon, error) {
	if cfg.BeastAddr == "" {
		return nil, fmt.Errorf("BeastAddr is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize database
	db, err := database.New(cfg.DBPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Create message channel for Beast format
	messageChan := make(chan *models.BeastMessage, 1000) // Buffered channel

	// Create scheduler
	sched := scheduler.New(ctx)

	// Create Beast client
	beastClient := dump1090.NewBeastClient(cfg.BeastAddr)

	// Configure batch settings
	batchSize := 100
	if cfg.BatchSize > 0 {
		batchSize = cfg.BatchSize
	}
	batchTimeout := 5 * time.Second
	if cfg.BatchTimeout > 0 {
		batchTimeout = time.Duration(cfg.BatchTimeout) * time.Second
	}

	// Create Beast collector task
	beastCollector := tasks.NewBeastCollectorTask(db, messageChan, batchSize, batchTimeout)
	sched.AddTask(beastCollector)

	// Start Beast message streamer in background
	go func() {
		if err := beastClient.StreamMessages(ctx, messageChan); err != nil {
			if ctx.Err() == nil { // Only log if not cancelled
				slog.Error("Beast streamer stopped", "error", err)
			}
		}
		// Only close channel if context was cancelled (graceful shutdown)
		// Otherwise, BeastClient should keep reconnecting
		select {
		case <-ctx.Done():
			close(messageChan)
		default:
			// StreamMessages returned but context not cancelled - this shouldn't happen
			// but if it does, close the channel
			close(messageChan)
		}
	}()

	return &Daemon{
		ctx:         ctx,
		cancel:      cancel,
		scheduler:   sched,
		database:    db,
		beastClient: beastClient,
		messageChan: messageChan,
		done:        make(chan struct{}),
	}, nil
}

func (d *Daemon) Start() error {
	slog.Info("Starting daemon")

	d.scheduler.Start()

	// Wait for context cancellation
	go func() {
		<-d.ctx.Done()
		close(d.done)
	}()

	slog.Info("Daemon started successfully")
	return nil
}

// Stop gracefully stops the daemon
func (d *Daemon) Stop() error {
	slog.Info("Stopping daemon")
	d.cancel()
	<-d.done

	d.scheduler.Stop()

	if err := d.beastClient.Close(); err != nil {
		slog.Error("Error closing Beast client", "error", err)
	}

	if err := d.database.Close(); err != nil {
		slog.Error("Error closing database", "error", err)
	}

	slog.Info("Daemon stopped")
	return nil
}
