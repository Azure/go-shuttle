package shuttle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProcessorWaitForInFlightHandlers_SkipsWaitWhenDisabled(t *testing.T) {
	for _, tc := range []struct {
		name        string
		gracePeriod *time.Duration
	}{
		{name: "unset"},
		{name: "zero", gracePeriod: durationPtr(0)},
		{name: "negative", gracePeriod: durationPtr(-time.Second)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			processor := newShutdownTestProcessor(tc.gracePeriod)
			processor.concurrencyTokens <- struct{}{}

			err := processor.waitForInFlightHandlers()

			require.NoError(t, err)
			require.Equal(t, 1, len(processor.concurrencyTokens))
		})
	}
}

func TestProcessorWaitForInFlightHandlers_WaitsForTokensToRelease(t *testing.T) {
	processor := newShutdownTestProcessor(durationPtr(time.Second))
	processor.concurrencyTokens <- struct{}{}

	errCh := make(chan error, 1)
	go func() { errCh <- processor.waitForInFlightHandlers() }()

	assertNoShutdownResult(t, errCh)
	<-processor.concurrencyTokens

	require.NoError(t, waitForShutdownResult(t, errCh))
}

func TestProcessorWaitForInFlightHandlers_TimesOutWithTokensStillHeld(t *testing.T) {
	shutdownGracePeriod := 25 * time.Millisecond
	processor := newShutdownTestProcessor(&shutdownGracePeriod)
	processor.concurrencyTokens <- struct{}{}

	start := time.Now()
	err := processor.waitForInFlightHandlers()
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "timed out waiting")
	require.Equal(t, 1, len(processor.concurrencyTokens))
	require.GreaterOrEqual(t, elapsed, shutdownGracePeriod)
	require.Less(t, elapsed, time.Second)
}

func newShutdownTestProcessor(shutdownGracePeriod *time.Duration) *Processor {
	return &Processor{
		options: ProcessorOptions{
			ShutdownGracePeriod: shutdownGracePeriod,
		},
		concurrencyTokens: make(chan struct{}, 1),
	}
}

func durationPtr(duration time.Duration) *time.Duration {
	return &duration
}

func assertNoShutdownResult(t *testing.T, errCh <-chan error) {
	t.Helper()

	select {
	case err := <-errCh:
		t.Fatalf("shutdown returned early: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
}

func waitForShutdownResult(t *testing.T, errCh <-chan error) error {
	t.Helper()

	select {
	case err := <-errCh:
		return err
	case <-time.After(time.Second):
		t.Fatal("shutdown did not return")
		return nil
	}
}
