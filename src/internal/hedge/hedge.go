package hedge

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Context causes an attempt sees when the hedge, not the caller, cuts it short.
var (
	ErrAborted = errors.New("aborted after another attempt won")
	ErrTimeout = errors.New("attempt timed out")
)

type Timing struct {
	MaxAttempts int
	// Attempt N+1 launches once attempt N fails or has been pending this long.
	Stagger time.Duration
	// Once any attempt wins, a straggler is aborted at whichever is later: this
	// long past the win, or Stagger past its own launch.
	AbortGrace     time.Duration
	AttemptTimeout time.Duration
}

type Outcome[T any] struct {
	// In launch order.
	Results []T
	winner  int
}

// Winner returns the first attempt to succeed, if any did.
func (o Outcome[T]) Winner() (T, bool) {
	if o.winner == -1 {
		var none T
		return none, false
	}
	return o.Results[o.winner], true
}

func (o Outcome[T]) IsWinner(index int) bool { return index == o.winner }

type handle[T any] struct {
	launched time.Time
	cancel   context.CancelCauseFunc
	done     chan struct{}
	result   T
}

// Run hedges attempt across up to MaxAttempts staggered parallel calls and
// waits for every launched attempt to settle before returning. attempt
// reports success with its bool, and must return promptly once its context
// is done; context.Cause tells it why.
func Run[T any](ctx context.Context, timing Timing, attempt func(ctx context.Context, num int) (T, bool)) Outcome[T] {
	var (
		mu      sync.Mutex
		winner  = -1
		winAt   time.Time
		won     = make(chan struct{})
		handles []*handle[T]
	)
	launch := func() {
		index := len(handles)
		attemptCtx, cancel := context.WithCancelCause(ctx)
		timeoutCtx, cancelTimeout := context.WithTimeoutCause(attemptCtx, timing.AttemptTimeout, ErrTimeout)
		h := &handle[T]{launched: time.Now(), cancel: cancel, done: make(chan struct{})}
		handles = append(handles, h)
		go func() {
			defer close(h.done)
			defer cancelTimeout()
			result, ok := attempt(timeoutCtx, index+1)
			h.result = result
			if !ok {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if winner == -1 {
				winner, winAt = index, time.Now()
				close(won)
			}
		}()
	}

	launch()
	for len(handles) < timing.MaxAttempts {
		stagger := time.NewTimer(timing.Stagger)
		select {
		case <-handles[len(handles)-1].done:
		case <-stagger.C:
		case <-won:
		case <-ctx.Done():
		}
		stagger.Stop()
		if isClosed(won) || ctx.Err() != nil {
			break
		}
		launch()
	}

	for _, h := range handles {
		select {
		case <-h.done:
			continue
		case <-won:
		}
		mu.Lock()
		deadline := later(winAt.Add(timing.AbortGrace), h.launched.Add(timing.Stagger))
		mu.Unlock()
		abort := time.NewTimer(time.Until(deadline))
		select {
		case <-h.done:
		case <-abort.C:
			h.cancel(ErrAborted)
			<-h.done
		}
		abort.Stop()
	}
	for _, h := range handles {
		h.cancel(nil)
	}

	results := make([]T, len(handles))
	for i, h := range handles {
		results[i] = h.result
	}
	mu.Lock()
	defer mu.Unlock()
	return Outcome[T]{Results: results, winner: winner}
}

func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
