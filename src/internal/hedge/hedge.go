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

// errAborted is the context cause attempts see when another attempt aborted the race.
var errAborted = errors.New("race aborted")

// Race is a hedged set of attempts. Run returns it as soon as one attempt
// wins, one aborts, or every attempt has failed; stragglers of a won race
// keep settling in the background.
type Race[T any] struct {
	mu        sync.Mutex
	winner    int
	winResult T
	won       chan struct{}
	aborted   chan struct{}
	abort     context.CancelCauseFunc
	settled   chan struct{}
	handles   []*handle[T]
}

type handle[T any] struct {
	done   chan struct{}
	result T
}

// Run hedges attempt across up to MaxAttempts staggered parallel calls.
// attempt reports how it settled, and must return promptly once its context
// is done; context.Cause tells it why.
func Run[T any](ctx context.Context, timing Timing, attempt func(ctx context.Context, num int) (T, Verdict)) *Race[T] {
	raceCtx, abort := context.WithCancelCause(ctx)
	r := &Race[T]{winner: -1, won: make(chan struct{}), aborted: make(chan struct{}), abort: abort, settled: make(chan struct{})}
	go func() {
		defer close(r.settled)
		defer abort(nil)
		r.launchAll(raceCtx, timing, attempt)
		for _, h := range r.handles {
			<-h.done
		}
	}()
	select {
	case <-r.won:
	case <-r.aborted:
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

func (r *Race[T]) launchAll(ctx context.Context, timing Timing, attempt func(ctx context.Context, num int) (T, Verdict)) {
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

func (r *Race[T]) launch(ctx context.Context, timing Timing, attempt func(ctx context.Context, num int) (T, Verdict)) {
	index := len(r.handles)
	timeoutCtx, cancelTimeout := context.WithTimeoutCause(ctx, timing.AttemptTimeout, ErrTimeout)
	h := &handle[T]{done: make(chan struct{})}
	r.handles = append(r.handles, h)
	go func() {
		defer close(h.done)
		defer cancelTimeout()
		result, verdict := attempt(timeoutCtx, index+1)
		h.result = result
		r.mu.Lock()
		defer r.mu.Unlock()
		switch verdict {
		case Won:
			if r.winner == -1 && !isClosed(r.aborted) {
				r.winner, r.winResult = index, result
				close(r.won)
			}
		case Aborted:
			if !isClosed(r.aborted) {
				close(r.aborted)
				r.abort(errAborted)
			}
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
