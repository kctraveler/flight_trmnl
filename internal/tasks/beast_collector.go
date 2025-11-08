package tasks

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"flight_trmnl/internal/database"
	"flight_trmnl/internal/models"
)

// BeastCollectorTask collects Beast format messages and stores them in batches
type BeastCollectorTask struct {
	repo         database.Repository
	messageChan  <-chan *models.BeastMessage
	batchSize    int
	batchTimeout time.Duration
	interval     time.Duration
}

// NewBeastCollectorTask creates a new Beast format collector task
func NewBeastCollectorTask(repo database.Repository, messageChan <-chan *models.BeastMessage, batchSize int, batchTimeout time.Duration) *BeastCollectorTask {
	return &BeastCollectorTask{
		repo:         repo,
		messageChan:  messageChan,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		interval:     1 * time.Second, // Not used for streaming, but required by interface
	}
}

// Name returns the task name
func (t *BeastCollectorTask) Name() string {
	return "BeastCollector"
}

// Interval returns the task execution interval (not used for streaming)
func (t *BeastCollectorTask) Interval() time.Duration {
	return t.interval
}

// Run processes Beast messages in batches for efficient database writes
func (t *BeastCollectorTask) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		t.processMessages(ctx)
	}()

	wg.Wait()
	return nil
}

// processMessages collects messages and writes them in batches
func (t *BeastCollectorTask) processMessages(ctx context.Context) {
	batch := make([]*models.BeastMessage, 0, t.batchSize)
	ticker := time.NewTicker(t.batchTimeout)
	defer ticker.Stop()

	flushBatch := func() {
		if len(batch) > 0 {
			if err := t.repo.InsertBeastMessagesBatch(batch); err != nil {
				log.Printf("Error inserting batch of %d messages: %v", len(batch), err)
			} else {
				log.Printf("Inserted batch of %d Beast messages", len(batch))
			}
			batch = batch[:0] // Reset slice but keep capacity
		}
	}

	for {
		select {
		case <-ctx.Done():
			// Flush any remaining messages before exiting
			flushBatch()
			return

		case msg := <-t.messageChan:
			if msg == nil {
				// Channel closed
				flushBatch()
				return
			}

			batch = append(batch, msg)

			// Flush when batch is full
			if len(batch) >= t.batchSize {
				flushBatch()
				ticker.Reset(t.batchTimeout) // Reset timer
			}

		case <-ticker.C:
			// Flush periodically even if batch isn't full
			flushBatch()
		}
	}
}

