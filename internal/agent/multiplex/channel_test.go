package multiplex

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestChannel_SendRecv(t *testing.T) {
	ch := NewChannel(0) // unbuffered

	intent := Intent{
		ID:     "test-1",
		TaskID: "task-1",
		Role:   RoleAnalyzer,
		Goal:   "Analyze this code",
	}

	// Send in goroutine (would block on unbuffered otherwise)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if !ch.SendIntent(intent) {
			t.Error("SendIntent failed")
		}
	}()

	// Receive
	received, err := ch.RecvIntent()
	if err != nil {
		t.Fatalf("RecvIntent failed: %v", err)
	}

	if received.ID != intent.ID {
		t.Errorf("expected ID %s, got %s", intent.ID, received.ID)
	}
	if received.Role != RoleAnalyzer {
		t.Errorf("expected role %s, got %s", RoleAnalyzer, received.Role)
	}

	wg.Wait()
	ch.Close()
}

func TestChannel_Buffered(t *testing.T) {
	ch := NewChannel(10) // buffered

	// Send multiple intents
	for i := 0; i < 5; i++ {
		intent := Intent{
			ID:     "test-" + string(rune('0'+i)),
			TaskID: "task-1",
			Role:   RoleAnalyzer,
			Goal:   "Test goal",
		}
		if !ch.SendIntent(intent) {
			t.Errorf("SendIntent %d failed", i)
		}
	}

	// Receive all
	for i := 0; i < 5; i++ {
		received, err := ch.RecvIntent()
		if err != nil {
			t.Fatalf("RecvIntent %d failed: %v", i, err)
		}
		if received.ID != "test-"+string(rune('0'+i)) {
			t.Errorf("expected ID test-%c, got %s", '0'+i, received.ID)
		}
	}

	ch.Close()
}

func TestChannel_Close(t *testing.T) {
	ch := NewChannel(0)

	ch.Close()

	if !ch.Closed() {
		t.Error("expected Closed() to return true")
	}

	// Recv should return context canceled
	_, err := ch.RecvIntent()
	if err == nil {
		t.Error("expected error after close")
	}
}

func TestChannel_NonBlocking(t *testing.T) {
	ch := NewChannel(1) // buffer size 1

	// Fill the buffer
	intent := Intent{ID: "test-1", TaskID: "task-1", Role: RoleAnalyzer}
	if !ch.SendIntent(intent) {
		t.Error("first send should succeed")
	}

	// This should fail (non-blocking, buffer full)
	intent2 := Intent{ID: "test-2", TaskID: "task-1", Role: RoleAnalyzer}
	if ch.SendIntent(intent2) {
		t.Error("second send should fail (buffer full)")
	}

	ch.Close()
}

func TestChannel_ContextCancel(t *testing.T) {
	// Create channel with an external context we can cancel
	externalCtx, cancel := context.WithCancel(context.Background())

	// Create a channel that we'll cancel via its internal context
	ch := NewChannel(0)

	// Cancel the channel's internal context
	ch.Close()

	// Recv should fail due to channel being closed
	_, err := ch.RecvIntent()
	if err == nil {
		t.Error("expected error after close")
	}

	// Clean up
	cancel()
	_ = externalCtx
}

func TestChannel_ResultRoundTrip(t *testing.T) {
	ch := NewChannel(0)

	result := Result{
		IntentID: "test-1",
		TaskID:   "task-1",
		Status:   StatusSuccess,
		Output:  "Analysis complete",
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ch.SendResult(result)
	}()

	received, err := ch.RecvResult()
	if err != nil {
		t.Fatalf("RecvResult failed: %v", err)
	}

	if received.Status != StatusSuccess {
		t.Errorf("expected status %s, got %s", StatusSuccess, received.Status)
	}
	if received.Output != "Analysis complete" {
		t.Errorf("expected output 'Analysis complete', got %s", received.Output)
	}

	wg.Wait()
	ch.Close()
}

func TestChannel_MultipleWorkers(t *testing.T) {
	ch := NewChannel(10)

	intents := []Intent{
		{ID: "1", TaskID: "task-1", Role: RoleAnalyzer},
		{ID: "2", TaskID: "task-1", Role: RoleEditor},
		{ID: "3", TaskID: "task-1", Role: RoleReviewer},
	}

	// Send from multiple goroutines
	var sendWg sync.WaitGroup
	for _, intent := range intents {
		sendWg.Add(1)
		go func(i Intent) {
			defer sendWg.Done()
			ch.SendIntent(i)
		}(intent)
	}

	// Receive from single goroutine
	received := make([]Intent, 0, 3)
	for i := 0; i < 3; i++ {
		intent, err := ch.RecvIntent()
		if err != nil {
			t.Fatalf("RecvIntent failed: %v", err)
		}
		received = append(received, intent)
	}

	if len(received) != 3 {
		t.Errorf("expected 3 intents, got %d", len(received))
	}

	sendWg.Wait()
	ch.Close()
}

func TestChannel_Deadline(t *testing.T) {
	ch := NewChannel(0)
	ch.Close()

	// Receive should fail immediately since channel is closed
	done := make(chan struct{})
	go func() {
		_, err := ch.RecvIntent()
		if err == nil {
			t.Error("expected error on closed channel")
		}
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(200 * time.Millisecond):
		t.Error("timeout took too long")
	}
}
