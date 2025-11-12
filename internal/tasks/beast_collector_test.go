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

func (m *mockRepository) Insert(msg *models.BeastMessage) error {
	m.messages = append(m.messages, msg)
	if len(m.errors) > 0 {
		err := m.errors[0]
		m.errors = m.errors[1:]
		return err
	}
	return nil
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

func (m *mockRepository) Close() error {
	return nil
}

func TestNewBeastCollectorTask(t *testing.T) {
	repo := &mockRepository{}
	messageChan := make(chan *models.BeastMessage, 10)
	batchSize := 10
	batchTimeout := 1 * time.Second

	task := NewBeastCollectorTask(repo, messageChan, batchSize, batchTimeout)

	require.NotNil(t, task)
	assert.Equal(t, "BeastCollector", task.Name())
	assert.Equal(t, 1*time.Second, task.Interval())
}

func TestBeastCollectorTask_BatchFlush(t *testing.T) {
	repo := &mockRepository{}
	messageChan := make(chan *models.BeastMessage, 100)
	batchSize := 5
	batchTimeout := 100 * time.Millisecond

	task := NewBeastCollectorTask(repo, messageChan, batchSize, batchTimeout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the task in a goroutine
	go func() {
		_ = task.Run(ctx)
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

func TestBeastCollectorTask_TimeoutFlush(t *testing.T) {
	repo := &mockRepository{}
	messageChan := make(chan *models.BeastMessage, 100)
	batchSize := 10
	batchTimeout := 100 * time.Millisecond

	task := NewBeastCollectorTask(repo, messageChan, batchSize, batchTimeout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the task in a goroutine
	go func() {
		_ = task.Run(ctx)
	}()

	// Send fewer messages than batch size
	msg := &models.BeastMessage{
		ICAO:        "TEST01",
		MessageType: "test",
	}
	messageChan <- msg

	// Wait for timeout
	time.Sleep(200 * time.Millisecond)

	// Check that message was inserted (timeout flush)
	assert.GreaterOrEqual(t, len(repo.messages), 1)
}

func TestBeastCollectorTask_ContextCancellation(t *testing.T) {
	repo := &mockRepository{}
	messageChan := make(chan *models.BeastMessage, 100)
	batchSize := 10
	batchTimeout := 1 * time.Second

	task := NewBeastCollectorTask(repo, messageChan, batchSize, batchTimeout)

	ctx, cancel := context.WithCancel(context.Background())

	// Start the task in a goroutine
	done := make(chan struct{})
	go func() {
		_ = task.Run(ctx)
		close(done)
	}()

	// Send a message
	msg := &models.BeastMessage{
		ICAO:        "TEST01",
		MessageType: "test",
	}
	messageChan <- msg

	// Cancel context
	cancel()

	// Wait for task to finish
	select {
	case <-done:
		// Task should have flushed remaining messages and exited
		assert.GreaterOrEqual(t, len(repo.messages), 1)
	case <-time.After(2 * time.Second):
		t.Fatal("Task did not exit after context cancellation")
	}
}

func TestBeastCollectorTask_ChannelClosed(t *testing.T) {
	repo := &mockRepository{}
	messageChan := make(chan *models.BeastMessage, 100)
	batchSize := 10
	batchTimeout := 1 * time.Second

	task := NewBeastCollectorTask(repo, messageChan, batchSize, batchTimeout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the task in a goroutine
	done := make(chan struct{})
	go func() {
		_ = task.Run(ctx)
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

	// Wait for task to finish
	select {
	case <-done:
		// Task should have flushed remaining messages and exited
		assert.GreaterOrEqual(t, len(repo.messages), 1)
	case <-time.After(2 * time.Second):
		t.Fatal("Task did not exit after channel closed")
	}
}

func TestBeastCollectorTask_InsertError(t *testing.T) {
	repo := &mockRepository{
		errors: []error{assert.AnError},
	}
	messageChan := make(chan *models.BeastMessage, 100)
	batchSize := 5
	batchTimeout := 100 * time.Millisecond

	task := NewBeastCollectorTask(repo, messageChan, batchSize, batchTimeout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the task in a goroutine
	go func() {
		_ = task.Run(ctx)
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

	// Task should continue running despite error
	// Messages should still be in the mock (even though insert failed)
	assert.GreaterOrEqual(t, len(repo.messages), batchSize)
}

