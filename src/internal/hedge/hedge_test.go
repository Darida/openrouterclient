package hedge

import (
	"context"
	"errors"
	"testing"
	"time"
)

var fastTiming = Timing{MaxAttempts: 3, Stagger: 5 * time.Second, AbortGrace: 20 * time.Millisecond, AttemptTimeout: 10 * time.Second}

func TestRun_whenFirstAttemptFails_thenSecondLaunchesBeforeStagger(t *testing.T) {
	// Arrange
	attempt := func(ctx context.Context, num int) (int, bool) { return num, num == 2 }
	start := time.Now()

	// Act
	outcome := Run(context.Background(), fastTiming, attempt).Settled()

	// Assert
	if !outcome.IsWinner(1) || time.Since(start) >= fastTiming.Stagger {
		t.Fatalf("outcome %+v after %s; want attempt 2 to win before the stagger", outcome, time.Since(start))
	}
}

func TestRun_whenStragglerOutlivesGrace_thenItSeesErrAborted(t *testing.T) {
	// Arrange
	timing := Timing{MaxAttempts: 2, Stagger: 30 * time.Millisecond, AbortGrace: 20 * time.Millisecond, AttemptTimeout: 10 * time.Second}
	attempt := func(ctx context.Context, num int) (error, bool) {
		if num == 2 {
			return nil, true
		}
		<-ctx.Done()
		return context.Cause(ctx), false
	}

	// Act
	outcome := Run(context.Background(), timing, attempt).Settled()

	// Assert
	if !errors.Is(outcome.Results[0], ErrAborted) {
		t.Fatalf("straggler cause = %v, want ErrAborted", outcome.Results[0])
	}
}

func TestRun_whenAttemptExceedsTimeout_thenItSeesErrTimeout(t *testing.T) {
	// Arrange
	timing := Timing{MaxAttempts: 1, Stagger: time.Second, AbortGrace: time.Second, AttemptTimeout: 20 * time.Millisecond}
	attempt := func(ctx context.Context, num int) (error, bool) {
		<-ctx.Done()
		return context.Cause(ctx), false
	}

	// Act
	outcome := Run(context.Background(), timing, attempt).Settled()

	// Assert
	if !errors.Is(outcome.Results[0], ErrTimeout) {
		t.Fatalf("cause = %v, want ErrTimeout", outcome.Results[0])
	}
}

func TestRun_whenEveryAttemptFails_thenNoWinner(t *testing.T) {
	// Arrange
	attempt := func(ctx context.Context, num int) (int, bool) { return num, false }

	// Act
	race := Run(context.Background(), fastTiming, attempt)

	// Assert
	if _, won := race.Winner(); won || len(race.Settled().Results) != 3 {
		t.Fatalf("got %+v; want 3 results and no winner", race.Settled())
	}
}

func TestRun_whenAttemptWinsWhileStragglerRuns_thenReturnsBeforeStragglerSettles(t *testing.T) {
	// Arrange
	timing := Timing{MaxAttempts: 2, Stagger: 20 * time.Millisecond, AbortGrace: 2 * time.Second, AttemptTimeout: 10 * time.Second}
	attempt := func(ctx context.Context, num int) (int, bool) {
		if num == 2 {
			return num, true
		}
		<-ctx.Done()
		return num, false
	}
	start := time.Now()

	// Act
	race := Run(context.Background(), timing, attempt)

	// Assert
	if winner, won := race.Winner(); !won || winner != 2 || time.Since(start) >= timing.AbortGrace {
		t.Fatalf("winner=%d won=%v after %s; want attempt 2 before the straggler's grace ends", winner, won, time.Since(start))
	}
}
