package daemon

import (
	"context"
	"fmt"
	"log"
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
				log.Printf("Beast streamer error: %v", err)
			}
		}
		close(messageChan)
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

// Start begins the daemon's main loop
func (d *Daemon) Start() error {
	log.Println("Starting daemon...")

	// Start the scheduler
	d.scheduler.Start()

	// Wait for context cancellation
	go func() {
		<-d.ctx.Done()
		close(d.done)
	}()

	log.Println("Daemon started successfully")
	return nil
}

// Stop gracefully stops the daemon
func (d *Daemon) Stop() error {
	log.Println("Stopping daemon...")
	d.cancel()
	<-d.done

	// Stop scheduler
	d.scheduler.Stop()

	// Close Beast client
	if err := d.beastClient.Close(); err != nil {
		log.Printf("Error closing Beast client: %v", err)
	}

	// Close database
	if err := d.database.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}

	log.Println("Daemon stopped")
	return nil
}

