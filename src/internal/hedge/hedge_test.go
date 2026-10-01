package hedge

import (
	"context"
	"errors"
	"testing"
	"time"
)

var fastTiming = Timing{MaxAttempts: 3, Stagger: 5 * time.Second, AttemptTimeout: 10 * time.Second}

func TestRun_whenFirstAttemptFails_thenSecondLaunchesBeforeStagger(t *testing.T) {
	// Arrange
	attempt := func(ctx context.Context, num int) (int, Verdict) {
		if num == 2 {
			return num, Won
		}
		return num, Lost
	}
	start := time.Now()

	// Act
	outcome := Run(context.Background(), fastTiming, attempt).Settled()

	// Assert
	if !outcome.IsWinner(1) || time.Since(start) >= fastTiming.Stagger {
		t.Fatalf("outcome %+v after %s; want attempt 2 to win before the stagger", outcome, time.Since(start))
	}
}

func TestRun_whenAnotherAttemptWins_thenStragglerRunsToItsOwnTimeout(t *testing.T) {
	// Arrange
	timing := Timing{MaxAttempts: 2, Stagger: 20 * time.Millisecond, AttemptTimeout: 100 * time.Millisecond}
	attempt := func(ctx context.Context, num int) (error, Verdict) {
		if num == 2 {
			return nil, Won
		}
		<-ctx.Done()
		return context.Cause(ctx), Lost
	}

	// Act
	outcome := Run(context.Background(), timing, attempt).Settled()

	// Assert
	if !errors.Is(outcome.Results[0], ErrTimeout) {
		t.Fatalf("straggler cause = %v, want ErrTimeout", outcome.Results[0])
	}
}

func TestRun_whenAttemptExceedsTimeout_thenItSeesErrTimeout(t *testing.T) {
	// Arrange
	timing := Timing{MaxAttempts: 1, Stagger: time.Second, AttemptTimeout: 20 * time.Millisecond}
	attempt := func(ctx context.Context, num int) (error, Verdict) {
		<-ctx.Done()
		return context.Cause(ctx), Lost
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
	attempt := func(ctx context.Context, num int) (int, Verdict) { return num, Lost }

	// Act
	race := Run(context.Background(), fastTiming, attempt)

	// Assert
	if _, won := race.Winner(); won || len(race.Settled().Results) != 3 {
		t.Fatalf("got %+v; want 3 results and no winner", race.Settled())
	}
}

func TestRun_whenAttemptWinsWhileStragglerRuns_thenReturnsBeforeStragglerSettles(t *testing.T) {
	// Arrange
	timing := Timing{MaxAttempts: 2, Stagger: 20 * time.Millisecond, AttemptTimeout: 2 * time.Second}
	attempt := func(ctx context.Context, num int) (int, Verdict) {
		if num == 2 {
			return num, Won
		}
		<-ctx.Done()
		return num, Lost
	}
	start := time.Now()

	// Act
	race := Run(context.Background(), timing, attempt)

	// Assert
	if winner, won := race.Winner(); !won || winner != 2 || time.Since(start) >= timing.AttemptTimeout {
		t.Fatalf("winner=%d won=%v after %s; want attempt 2 before the straggler times out", winner, won, time.Since(start))
	}
}

func TestRun_whenAttemptAborts_thenNoFurtherAttemptLaunches(t *testing.T) {
	// Arrange
	attempt := func(ctx context.Context, num int) (int, Verdict) { return num, Aborted }

	// Act
	race := Run(context.Background(), fastTiming, attempt)

	// Assert
	if len(race.Settled().Results) != 1 {
		t.Fatalf("got %+v; want only the aborting attempt", race.Settled())
	}
}

func TestRun_whenAttemptAborts_thenInFlightAttemptIsCanceled(t *testing.T) {
	// Arrange
	timing := Timing{MaxAttempts: 2, Stagger: 10 * time.Millisecond, AttemptTimeout: 2 * time.Second}
	attempt := func(ctx context.Context, num int) (error, Verdict) {
		if num == 2 {
			return nil, Aborted
		}
		<-ctx.Done()
		return context.Cause(ctx), Lost
	}

	// Act
	outcome := Run(context.Background(), timing, attempt).Settled()

	// Assert
	if !errors.Is(outcome.Results[0], errAborted) {
		t.Fatalf("in-flight cause = %v, want errAborted", outcome.Results[0])
	}
}
