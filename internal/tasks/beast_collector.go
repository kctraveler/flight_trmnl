package tasks

import (
	"context"
	"log/slog"
	"time"

	"flight_trmnl/internal/database"
	"flight_trmnl/internal/models"
)

// BeastCollector collects Beast format messages and commits them to the database in batches
type BeastCollector struct {
	repo          database.BeastMessageRepository
	messageChan   <-chan *models.BeastMessage
	batchSize     int           // maximum number of messages in a batch before committing to database
	flushInterval time.Duration // time to flush batch even if not full
}

// Default batch size is 100 messages and flush interval is 1 second
func NewBeastCollector(repo database.BeastMessageRepository, messageChan <-chan *models.BeastMessage) *BeastCollector {
	return &BeastCollector{
		repo:          repo,
		messageChan:   messageChan,
		batchSize:     100,
		flushInterval: 1 * time.Second,
	}
}

// NewBeastCollectorWithConfig creates a new Beast format collector with custom batch settings
func NewBeastCollectorWithConfig(repo database.BeastMessageRepository, messageChan <-chan *models.BeastMessage, batchSize int, flushInterval time.Duration) *BeastCollector {
	return &BeastCollector{
		repo:          repo,
		messageChan:   messageChan,
		batchSize:     batchSize,
		flushInterval: flushInterval,
	}
}

// Start begins collecting messages and writing them to the database in batches
// This method blocks until the context is cancelled or the message channel is closed
// Batches are flushed when they reach batchSize (100) or 1 second has passed since the last transaction
func (c *BeastCollector) Start(ctx context.Context) error {
	batch := make([]*models.BeastMessage, 0, c.batchSize)
	var lastFlushTime time.Time

	flushBatch := func() {
		if len(batch) > 0 {
			if err := c.repo.InsertBatch(batch); err != nil {
				slog.Error("Error inserting batch of messages", "batch_size", len(batch), "error", err)
			} else {
				lastFlushTime = time.Now()
				slog.Info("Inserted batch of Beast messages",
					"batch_size", len(batch),
				)
			}
			batch = batch[:0] // Reset slice but keep capacity
		}
	}

	// Initialize lastFlushTime to now so first message doesn't immediately flush
	lastFlushTime = time.Now()

	for {
		select {
		case <-ctx.Done():
			// Flush any remaining messages before exiting
			flushBatch()
			return ctx.Err()

		case msg, ok := <-c.messageChan:
			if !ok {
				// Channel closed, flush any remaining messages
				flushBatch()
				return nil
			}

			if msg == nil {
				continue
			}

			batch = append(batch, msg)

			// Log debug information about the message and batch
			slog.Debug("Added message to batch",
				"icao", msg.ICAO,
				"message_type", msg.MessageType,
				"signal_level", msg.SignalLevel,
				"timestamp", msg.Timestamp.Format(time.RFC3339Nano),
				"current_batch_size", len(batch),
				"max_batch_size", c.batchSize,
			)

			// Flush when batch is full
			if len(batch) >= c.batchSize {
				flushBatch()
			} else {
				// Check if 1 second has passed since last transaction
				if time.Since(lastFlushTime) >= c.flushInterval {
					flushBatch()
				}
			}
		}
	}
}
