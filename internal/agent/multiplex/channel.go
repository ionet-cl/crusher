// Package multiplex provides multi-agent communication primitives.
package multiplex

import "context"

// Channel provides typed channels for agent communication.
// No shared memory, no mutex - pure Go channels.
// This is the low-level primitive for agent-to-agent communication.
type Channel struct {
	intentC  chan Intent
	resultC  chan Result
	errC     chan error
	ctx      context.Context
	cancel   context.CancelFunc
	closed   bool
}

// NewChannel creates a new communication channel.
// bufSize controls the buffer for intents and results.
// A size of 0 means unbuffered (synchronous).
// Context controls lifecycle - cancelled when context is done.
func NewChannel(bufSize int) *Channel {
	if bufSize < 0 {
		bufSize = 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Channel{
		intentC: make(chan Intent, bufSize),
		resultC: make(chan Result, bufSize),
		errC:    make(chan error, bufSize),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// IntentChan returns the channel for sending/receiving intents.
func (c *Channel) IntentChan() <-chan Intent {
	return c.intentC
}

// ResultChan returns the channel for sending/receiving results.
func (c *Channel) ResultChan() <-chan Result {
	return c.resultC
}

// ErrChan returns the channel for errors.
func (c *Channel) ErrChan() <-chan error {
	return c.errC
}

// SendIntent sends an intent (non-blocking if buffered).
// Returns false if the channel is closed or blocked.
func (c *Channel) SendIntent(intent Intent) bool {
	select {
	case c.intentC <- intent:
		return true
	case <-c.ctx.Done():
		return false
	default:
		return false
	}
}

// SendResult sends a result (non-blocking if buffered).
func (c *Channel) SendResult(result Result) bool {
	select {
	case c.resultC <- result:
		return true
	case <-c.ctx.Done():
		return false
	default:
		return false
	}
}

// SendError sends an error (non-blocking if buffered).
func (c *Channel) SendError(err error) bool {
	if err == nil {
		return false
	}
	select {
	case c.errC <- err:
		return true
	case <-c.ctx.Done():
		return false
	default:
		return false
	}
}

// RecvIntent receives an intent. Blocking until context done or closed.
func (c *Channel) RecvIntent() (Intent, error) {
	select {
	case intent, ok := <-c.intentC:
		if !ok {
			return Intent{}, ErrChannelClosed
		}
		return intent, nil
	case <-c.ctx.Done():
		return Intent{}, c.ctx.Err()
	}
}

// RecvResult receives a result. Blocking until context done or closed.
func (c *Channel) RecvResult() (Result, error) {
	select {
	case result, ok := <-c.resultC:
		if !ok {
			return Result{}, ErrChannelClosed
		}
		return result, nil
	case <-c.ctx.Done():
		return Result{}, c.ctx.Err()
	}
}

// Close closes all channels and cancels context.
func (c *Channel) Close() {
	if c.closed {
		return
	}
	c.closed = true
	c.cancel()
	close(c.intentC)
	close(c.resultC)
	close(c.errC)
}

// Closed returns true if the channel has been closed.
func (c *Channel) Closed() bool {
	return c.closed
}

// ErrChannelClosed is returned when receiving from a closed channel.
var ErrChannelClosed = &ChannelError{"channel closed"}

// ChannelError represents a channel-level error.
type ChannelError struct {
	Msg string
}

func (e *ChannelError) Error() string {
	return e.Msg
}
