package hedge

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrTimeout is the context cause an attempt sees when its own time runs out.
var ErrTimeout = errors.New("attempt timed out")

type Timing struct {
	MaxAttempts int
	// Attempt N+1 launches once attempt N fails or has been pending this long.
	Stagger time.Duration
	// Every attempt gets this long, win or not; a straggler is never cut short.
	AttemptTimeout time.Duration
}

// Race is a hedged set of attempts. Run returns it as soon as one attempt
// wins or every attempt has failed; stragglers keep settling in the background.
type Race[T any] struct {
	mu        sync.Mutex
	winner    int
	winResult T
	won       chan struct{}
	settled   chan struct{}
	handles   []*handle[T]
}

type handle[T any] struct {
	done   chan struct{}
	result T
}

// Run hedges attempt across up to MaxAttempts staggered parallel calls.
// attempt reports success with its bool, and must return promptly once its
// context is done; context.Cause tells it why.
func Run[T any](ctx context.Context, timing Timing, attempt func(ctx context.Context, num int) (T, bool)) *Race[T] {
	r := &Race[T]{winner: -1, won: make(chan struct{}), settled: make(chan struct{})}
	go func() {
		defer close(r.settled)
		r.launchAll(ctx, timing, attempt)
		for _, h := range r.handles {
			<-h.done
		}
	}()
	select {
	case <-r.won:
	case <-r.settled:
	}
	return r
}

func (r *Race[T]) Winner() (T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.winResult, r.winner != -1
}

// Settled blocks until every launched attempt has finished and returns them all.
func (r *Race[T]) Settled() Outcome[T] {
	<-r.settled
	results := make([]T, len(r.handles))
	for i, h := range r.handles {
		results[i] = h.result
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return Outcome[T]{Results: results, winner: r.winner}
}

func (r *Race[T]) launchAll(ctx context.Context, timing Timing, attempt func(ctx context.Context, num int) (T, bool)) {
	r.launch(ctx, timing, attempt)
	for len(r.handles) < timing.MaxAttempts {
		stagger := time.NewTimer(timing.Stagger)
		select {
		case <-r.handles[len(r.handles)-1].done:
		case <-stagger.C:
		case <-r.won:
		case <-ctx.Done():
		}
		stagger.Stop()
		if isClosed(r.won) || ctx.Err() != nil {
			return
		}
		r.launch(ctx, timing, attempt)
	}
}

func (r *Race[T]) launch(ctx context.Context, timing Timing, attempt func(ctx context.Context, num int) (T, bool)) {
	index := len(r.handles)
	timeoutCtx, cancelTimeout := context.WithTimeoutCause(ctx, timing.AttemptTimeout, ErrTimeout)
	h := &handle[T]{done: make(chan struct{})}
	r.handles = append(r.handles, h)
	go func() {
		defer close(h.done)
		defer cancelTimeout()
		result, ok := attempt(timeoutCtx, index+1)
		h.result = result
		if !ok {
			return
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.winner == -1 {
			r.winner, r.winResult = index, result
			close(r.won)
		}
	}()
}

type Outcome[T any] struct {
	// In launch order.
	Results []T
	winner  int
}

func (o Outcome[T]) IsWinner(index int) bool { return index == o.winner }

func (o Outcome[T]) HasWinner() bool { return o.winner != -1 }

func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
