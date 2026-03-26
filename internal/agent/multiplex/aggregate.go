// Package multiplex provides multi-agent communication primitives.
package multiplex

import (
	"context"
	"sync"
	"time"
)

// Aggregator collects results from sub-agents and combines them.
type Aggregator struct {
	resultChan <-chan Result
	ctx       context.Context
	cancel    context.CancelFunc

	mu           sync.Mutex
	results      map[string][]Result // keyed by TaskID
	expectedCount map[string]int     // expected result count per TaskID
	completed    map[string]bool
	onComplete   func(taskID string, results []Result)
}

// NewAggregator creates a result aggregator.
func NewAggregator(ctx context.Context, resultChan <-chan Result) *Aggregator {
	aggCtx, cancel := context.WithCancel(ctx)
	return &Aggregator{
		resultChan:    resultChan,
		ctx:          aggCtx,
		cancel:       cancel,
		results:      make(map[string][]Result),
		expectedCount: make(map[string]int),
		completed:    make(map[string]bool),
	}
}

// SetExpectedCount tells the aggregator how many results to wait for a task.
func (a *Aggregator) SetExpectedCount(taskID string, count int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.expectedCount[taskID] = count
}

// OnComplete sets a callback for when a task is complete.
func (a *Aggregator) OnComplete(fn func(taskID string, results []Result)) {
	a.onComplete = fn
}

// Start begins collecting results. Non-blocking.
func (a *Aggregator) Start() {
	go a.run()
}

// Stop gracefully stops the aggregator.
func (a *Aggregator) Stop() {
	a.cancel()
}

func (a *Aggregator) run() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case result, ok := <-a.resultChan:
			if !ok {
				return
			}
			a.addResult(result)
		}
	}
}

func (a *Aggregator) addResult(result Result) {
	a.mu.Lock()

	taskID := result.TaskID
	a.results[taskID] = append(a.results[taskID], result)

	// Check if all results for this task are in
	expected, hasExpected := a.expectedCount[taskID]
	resultCount := len(a.results[taskID])
	alreadyCompleted := a.completed[taskID]

	// Determine if complete
	isComplete := false
	if hasExpected {
		isComplete = resultCount >= expected
	}

	if isComplete && !alreadyCompleted {
		a.completed[taskID] = true
		// Release lock before calling notifyComplete to avoid deadlock
		a.mu.Unlock()
		go a.notifyComplete(taskID)
	} else {
		a.mu.Unlock()
	}
}

func (a *Aggregator) isComplete(taskID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	results := a.results[taskID]
	if len(results) == 0 {
		return false
	}

	expected, hasExpected := a.expectedCount[taskID]

	// If we know the expected count, wait until we have all results
	if hasExpected {
		return len(results) >= expected
	}

	// Legacy fallback: if any result indicates all done
	for _, r := range results {
		if r.Status == StatusSuccess || r.Status == StatusPartial {
			return true
		}
	}
	return false
}

func (a *Aggregator) notifyComplete(taskID string) {
	a.mu.Lock()
	results := a.results[taskID]
	a.mu.Unlock()

	if a.onComplete != nil {
		a.onComplete(taskID, results)
	}
}

// WaitResult waits for results for a specific task.
func (a *Aggregator) WaitResult(taskID string, timeout time.Duration) ([]Result, bool) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		a.mu.Lock()
		results, exists := a.results[taskID]
		completed := a.completed[taskID]
		a.mu.Unlock()

		if exists && completed {
			return results, true
		}

		time.Sleep(10 * time.Millisecond)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	results, exists := a.results[taskID]
	return results, exists && a.completed[taskID]
}

// AggregateResult combines multiple results into a single Result.
func AggregateResult(results []Result, taskID string) Result {
	if len(results) == 0 {
		return Result{
			TaskID: taskID,
			Status: StatusFailed,
			Error:  &ResultError{Code: "NO_RESULTS", Message: "No results to aggregate"},
		}
	}

	if len(results) == 1 {
		return results[0]
	}

	// Combine outputs
	var combined Output
	for _, r := range results {
		combined.Summaries = append(combined.Summaries, Summary{
			WorkerID: r.IntentID,
			Output:   r.Output,
			Status:   string(r.Status),
		})
		if len(r.FilesModified) > 0 {
			combined.Files = append(combined.Files, r.FilesModified...)
		}
	}

	// Determine overall status
	status := StatusSuccess
	for _, r := range results {
		if r.Status == StatusFailed {
			status = StatusPartial
			break
		}
		if r.Status == StatusPartial {
			status = StatusPartial
		}
	}

	return Result{
		TaskID:         taskID,
		Status:         status,
		Output:         combined.String(),
		FilesModified:  combined.Files,
	}
}

// Output represents aggregated output from multiple agents.
type Output struct {
	Summaries []Summary
	Files     []string
}

type Summary struct {
	WorkerID string
	Output   string
	Status   string
}

func (o Output) String() string {
	var result string
	for _, s := range o.Summaries {
		result += s.Output + "\n"
	}
	return result
}
