package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Task interface for scheduled tasks
type Task interface {
	Run(ctx context.Context) error
	Interval() time.Duration
	Name() string
}

// Scheduler manages multiple scheduled tasks
type Scheduler struct {
	ctx    context.Context
	cancel context.CancelFunc
	tasks  []Task
	wg     sync.WaitGroup
}

// New creates a new task scheduler
func New(ctx context.Context) *Scheduler {
	ctx, cancel := context.WithCancel(ctx)
	return &Scheduler{
		ctx:    ctx,
		cancel: cancel,
		tasks:  make([]Task, 0),
	}
}

// AddTask adds a task to the scheduler
func (s *Scheduler) AddTask(task Task) {
	s.tasks = append(s.tasks, task)
}

// Start begins running all scheduled tasks
func (s *Scheduler) Start() {
	slog.Info("Starting task scheduler")
	for _, task := range s.tasks {
		s.wg.Add(1)
		go s.runTask(task)
	}
	slog.Info("Task scheduler started", "task_count", len(s.tasks))
}

// Stop gracefully stops all tasks
func (s *Scheduler) Stop() {
	slog.Info("Stopping task scheduler")
	s.cancel()
	s.wg.Wait()
	slog.Info("Task scheduler stopped")
}

// runTask runs a single task on its schedule
func (s *Scheduler) runTask(task Task) {
	defer s.wg.Done()

	ticker := time.NewTicker(task.Interval())
	defer ticker.Stop()

	// Run immediately on start
	if err := task.Run(s.ctx); err != nil {
		slog.Error("Error running task", "task", task.Name(), "error", err)
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := task.Run(s.ctx); err != nil {
				slog.Error("Error running task", "task", task.Name(), "error", err)
			}
		}
	}
}

