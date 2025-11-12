package tasks

import (
	"context"
	"testing"
	"time"

	"flight_trmnl/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRepository is a simple mock implementation of database.BeastMessageRepository
type mockRepository struct {
	messages []*models.BeastMessage
	errors   []error
}

func (m *mockRepository) InsertBatch(msgs []*models.BeastMessage) error {
	m.messages = append(m.messages, msgs...)
	if len(m.errors) > 0 {
		err := m.errors[0]
		m.errors = m.errors[1:]
		return err
	}
	return nil
}

func TestNewBeastCollector(t *testing.T) {
	repo := &mockRepository{}
	messageChan := make(chan *models.BeastMessage, 10)

	collector := NewBeastCollector(repo, messageChan)

	require.NotNil(t, collector)
	assert.Equal(t, 100, collector.batchSize)
	assert.Equal(t, 1*time.Second, collector.flushInterval)
}

func TestNewBeastCollectorWithConfig(t *testing.T) {
	repo := &mockRepository{}
	messageChan := make(chan *models.BeastMessage, 10)
	batchSize := 50
	flushInterval := 500 * time.Millisecond

	collector := NewBeastCollectorWithConfig(repo, messageChan, batchSize, flushInterval)

	require.NotNil(t, collector)
	assert.Equal(t, batchSize, collector.batchSize)
	assert.Equal(t, flushInterval, collector.flushInterval)
}

func TestBeastCollector_BatchFlush(t *testing.T) {
	repo := &mockRepository{}
	messageChan := make(chan *models.BeastMessage, 100)
	batchSize := 5
	flushInterval := 100 * time.Millisecond

	collector := NewBeastCollectorWithConfig(repo, messageChan, batchSize, flushInterval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the collector in a goroutine
	go func() {
		_ = collector.Start(ctx)
	}()

	// Send messages that should trigger a batch flush
	for i := 0; i < batchSize; i++ {
		msg := &models.BeastMessage{
			ICAO:        "TEST01",
			MessageType: "test",
		}
		messageChan <- msg
	}

	// Wait a bit for batch to be flushed
	time.Sleep(200 * time.Millisecond)

	// Check that messages were inserted
	assert.GreaterOrEqual(t, len(repo.messages), batchSize)
}

func TestBeastCollector_TimeoutFlush(t *testing.T) {
	repo := &mockRepository{}
	messageChan := make(chan *models.BeastMessage, 100)
	batchSize := 10
	flushInterval := 100 * time.Millisecond

	collector := NewBeastCollectorWithConfig(repo, messageChan, batchSize, flushInterval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the collector in a goroutine
	go func() {
		_ = collector.Start(ctx)
	}()

	// Send first message (starts the batch)
	msg1 := &models.BeastMessage{
		ICAO:        "TEST01",
		MessageType: "test",
	}
	messageChan <- msg1

	// Wait for flush interval to pass
	time.Sleep(flushInterval + 50*time.Millisecond)

	// Send a second message to trigger the timeout check
	// This will cause the collector to check time.Since(lastFlushTime) and flush
	msg2 := &models.BeastMessage{
		ICAO:        "TEST02",
		MessageType: "test",
	}
	messageChan <- msg2

	// Wait a bit for the flush to complete
	time.Sleep(50 * time.Millisecond)

	// Check that both messages were inserted (timeout flush should have triggered)
	assert.GreaterOrEqual(t, len(repo.messages), 2)
}

func TestBeastCollector_ContextCancellation(t *testing.T) {
	repo := &mockRepository{}
	messageChan := make(chan *models.BeastMessage, 100)
	batchSize := 10
	flushInterval := 1 * time.Second

	collector := NewBeastCollectorWithConfig(repo, messageChan, batchSize, flushInterval)

	ctx, cancel := context.WithCancel(context.Background())

	// Start the collector in a goroutine
	done := make(chan struct{})
	go func() {
		_ = collector.Start(ctx)
		close(done)
	}()

	// Send a message
	msg := &models.BeastMessage{
		ICAO:        "TEST01",
		MessageType: "test",
	}
	messageChan <- msg

	// Give collector time to process the message
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for collector to finish
	select {
	case <-done:
		// Collector should have flushed remaining messages and exited
		assert.GreaterOrEqual(t, len(repo.messages), 1)
	case <-time.After(2 * time.Second):
		t.Fatal("Collector did not exit after context cancellation")
	}
}

func TestBeastCollector_ChannelClosed(t *testing.T) {
	repo := &mockRepository{}
	messageChan := make(chan *models.BeastMessage, 100)
	batchSize := 10
	flushInterval := 1 * time.Second

	collector := NewBeastCollectorWithConfig(repo, messageChan, batchSize, flushInterval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the collector in a goroutine
	done := make(chan struct{})
	go func() {
		_ = collector.Start(ctx)
		close(done)
	}()

	// Send a message
	msg := &models.BeastMessage{
		ICAO:        "TEST01",
		MessageType: "test",
	}
	messageChan <- msg

	// Close channel
	close(messageChan)

	// Wait for collector to finish
	select {
	case <-done:
		// Collector should have flushed remaining messages and exited
		assert.GreaterOrEqual(t, len(repo.messages), 1)
	case <-time.After(2 * time.Second):
		t.Fatal("Collector did not exit after channel closed")
	}
}

func TestBeastCollector_InsertError(t *testing.T) {
	repo := &mockRepository{
		errors: []error{assert.AnError},
	}
	messageChan := make(chan *models.BeastMessage, 100)
	batchSize := 5
	flushInterval := 100 * time.Millisecond

	collector := NewBeastCollectorWithConfig(repo, messageChan, batchSize, flushInterval)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the collector in a goroutine
	go func() {
		_ = collector.Start(ctx)
	}()

	// Send messages that should trigger a batch flush
	for i := 0; i < batchSize; i++ {
		msg := &models.BeastMessage{
			ICAO:        "TEST01",
			MessageType: "test",
		}
		messageChan <- msg
	}

	// Wait a bit for batch to be flushed
	time.Sleep(200 * time.Millisecond)

	// Collector should continue running despite error
	// Messages should still be in the mock (even though insert failed)
	assert.GreaterOrEqual(t, len(repo.messages), batchSize)
}
