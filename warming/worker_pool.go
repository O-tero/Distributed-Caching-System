package warming

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// WorkerPool manages a pool of concurrent workers that execute warming tasks.
type WorkerPool struct {
	service    *Service
	workers    []*Worker
	taskQueue  chan WarmTask
	activeCount atomic.Int32
	mu         sync.RWMutex
	stopChan   chan struct{}
	wg         sync.WaitGroup
}

// Worker represents a single warming worker goroutine.
type Worker struct {
	id          int
	state       string // "idle", "busy", "stopped"
	currentKey  string
	startedAt   *time.Time
	mu          sync.RWMutex
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(service *Service, numWorkers int) *WorkerPool {
	pool := &WorkerPool{
		service:   service,
		workers:   make([]*Worker, numWorkers),
		taskQueue: make(chan WarmTask, 1000), // Buffered queue
		stopChan:  make(chan struct{}),
	}

	// Start workers
	for i := 0; i < numWorkers; i++ {
		worker := &Worker{
			id:    i,
			state: "idle",
		}
		pool.workers[i] = worker

		pool.wg.Add(1)
		go pool.runWorker(worker)
	}

	return pool
}

// QueueTasks adds tasks to the worker pool queue.
func (p *WorkerPool) QueueTasks(tasks []WarmTask) int {
	queued := 0
	for _, task := range tasks {
		select {
		case p.taskQueue <- task:
			queued++
		default:
			// Queue full, skip this task
			// TODO: In production, use persistent queue or apply backpressure
		}
	}
	return queued
}

// runWorker is the main worker loop.
func (p *WorkerPool) runWorker(worker *Worker) {
	defer p.wg.Done()

	for {
		select {
		case <-p.stopChan:
			worker.setState("stopped")
			return

		case task := <-p.taskQueue:
			// Execute task
			worker.startTask(task.Key)
			p.activeCount.Add(1)

			ctx, cancel := context.WithTimeout(context.Background(), p.service.config.OriginTimeout*2)
			err := p.service.ExecuteWarmTask(ctx, task)
			cancel()

			if err != nil {
				// Retry logic with exponential backoff
				p.retryTask(task)
			}

			worker.finishTask()
			p.activeCount.Add(-1)
		}
	}
}

// retryTask implements retry logic with exponential backoff.
func (p *WorkerPool) retryTask(task WarmTask) {
	maxRetries := p.service.config.RetryAttempts
	backoff := p.service.config.BackoffBase

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Exponential backoff with jitter
		sleepTime := backoff * time.Duration(1<<uint(attempt-1))
		jitter := time.Duration(time.Now().UnixNano()%int64(sleepTime/2))
		time.Sleep(sleepTime + jitter)

		ctx, cancel := context.WithTimeout(context.Background(), p.service.config.OriginTimeout*2)
		err := p.service.ExecuteWarmTask(ctx, task)
		cancel()

		if err == nil {
			return // Success
		}

		if attempt == maxRetries {
			// Final failure, give up
			p.service.publishWarmCompletion(task.Key, "failure", 0, task.Strategy)
		}
	}
}

// ActiveCount returns the number of currently active workers.
func (p *WorkerPool) ActiveCount() int {
	return int(p.activeCount.Load())
}

// QueueSize returns the number of tasks waiting in queue.
func (p *WorkerPool) QueueSize() int {
	return len(p.taskQueue)
}

// GetWorkerStatus returns status of all workers.
func (p *WorkerPool) GetWorkerStatus() []WorkerStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	status := make([]WorkerStatus, len(p.workers))
	for i, worker := range p.workers {
		worker.mu.RLock()
		status[i] = WorkerStatus{
			ID:         worker.id,
			State:      worker.state,
			CurrentKey: worker.currentKey,
			StartedAt:  worker.startedAt,
		}
		worker.mu.RUnlock()
	}

	return status
}

// Shutdown gracefully stops all workers.
func (p *WorkerPool) Shutdown() {
	close(p.stopChan)
	p.wg.Wait()
}

// Worker methods

func (w *Worker) startTask(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	w.state = "busy"
	w.currentKey = key
	w.startedAt = &now
}

func (w *Worker) finishTask() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.state = "idle"
	w.currentKey = ""
	w.startedAt = nil
}

func (w *Worker) setState(state string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.state = state
}