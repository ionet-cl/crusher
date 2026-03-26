// Package multiplex provides multi-agent communication primitives.
package multiplex

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// WorkerConfig configures a worker.
type WorkerConfig struct {
	// ID is unique identifier for this worker.
	ID string
	// IntentChan is the channel to receive intents from.
	IntentChan <-chan Intent
	// ResultChan is the channel to send results to.
	ResultChan chan<- Result
	// ProcessFunc does the actual work for an intent.
	// Returns the result. This is the only thing that varies between workers.
	ProcessFunc func(ctx context.Context, intent Intent) Result
}

// Worker processes intents from a channel and emits results.
type Worker struct {
	config WorkerConfig
	ctx    context.Context
	cancel  context.CancelFunc
	wg     sync.WaitGroup
}

// NewWorker creates a new worker.
func NewWorker(ctx context.Context, config WorkerConfig) *Worker {
	workerCtx, cancel := context.WithCancel(ctx)
	return &Worker{
		config: config,
		ctx:    workerCtx,
		cancel:  cancel,
	}
}

// Start begins processing intents. Non-blocking.
func (w *Worker) Start() {
	w.wg.Add(1)
	go w.run()
}

// Stop gracefully stops the worker.
func (w *Worker) Stop() {
	w.cancel()
	w.wg.Wait()
}

func (w *Worker) run() {
	defer w.wg.Done()
	slog.Debug("worker started", "id", w.config.ID)

	for {
		select {
		case <-w.ctx.Done():
			slog.Debug("worker stopping", "id", w.config.ID)
			return
		case intent, ok := <-w.config.IntentChan:
			if !ok {
				slog.Debug("worker: intent channel closed", "id", w.config.ID)
				return
			}
			w.processIntent(intent)
		}
	}
}

func (w *Worker) processIntent(intent Intent) {
	slog.Debug("worker processing", "id", w.config.ID, "intent", intent.ID)

	result := w.config.ProcessFunc(w.ctx, intent)

	select {
	case w.config.ResultChan <- result:
		// Sent successfully
	case <-w.ctx.Done():
		slog.Debug("worker: context cancelled while sending result", "id", w.config.ID)
	}
}

// Pool manages multiple workers.
type Pool struct {
	workers []*Worker
	config  PoolConfig
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// PoolConfig configures the worker pool.
type PoolConfig struct {
	// Size is the number of workers.
	Size int
	// IntentChan is the shared channel for receiving intents.
	IntentChan chan Intent
	// ResultChan is the shared channel for sending results.
	ResultChan chan Result
	// ProcessFunc is called by each worker to process intents.
	ProcessFunc func(ctx context.Context, intent Intent) Result
}

// NewPool creates a pool of workers.
func NewPool(ctx context.Context, config PoolConfig) *Pool {
	poolCtx, cancel := context.WithCancel(ctx)
	pool := &Pool{
		workers: make([]*Worker, 0, config.Size),
		config:  config,
		ctx:    poolCtx,
		cancel:  cancel,
	}

	for i := 0; i < config.Size; i++ {
		worker := NewWorker(poolCtx, WorkerConfig{
			ID:         fmt.Sprintf("worker-%d", i),
			IntentChan: config.IntentChan,
			ResultChan: config.ResultChan,
			ProcessFunc: config.ProcessFunc,
		})
		pool.workers = append(pool.workers, worker)
	}

	return pool
}

// Start starts all workers. Non-blocking.
func (p *Pool) Start() {
	for _, w := range p.workers {
		w.Start()
	}
}

// Stop gracefully stops all workers.
func (p *Pool) Stop() {
	p.cancel()
	p.wg.Wait()
	for _, w := range p.workers {
		w.Stop()
	}
}
