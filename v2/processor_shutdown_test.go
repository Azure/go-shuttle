package shuttle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
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

func TestProcessorStart_ShutdownGracePeriodWaitsWhenReceiveReturnsContextError(t *testing.T) {
	handler := newShutdownBlockedHandler()
	secondReceiveStarted := make(chan struct{})
	receiver := &contextErrorAfterFirstReceive{
		secondReceiveStarted: secondReceiveStarted,
	}
	shutdownGracePeriod := 200 * time.Millisecond

	processor := NewProcessor(receiver, handler.Handle, &ProcessorOptions{
		MaxConcurrency:      2,
		ReceiveInterval:     durationPtr(time.Millisecond),
		ShutdownGracePeriod: &shutdownGracePeriod,
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- processor.Start(ctx) }()
	handler.waitStarted(t, errCh)
	waitForSignal(t, secondReceiveStarted, errCh, "second receive did not start")

	cancel()
	assertProcessorStillRunning(t, errCh)
	handler.releaseAndWait(t)

	require.ErrorIs(t, waitForShutdownResult(t, errCh), context.Canceled)
}

func TestProcessorStart_CanceledShutdownTimeoutDoesNotRetry(t *testing.T) {
	handler := newShutdownBlockedHandler()
	receiver := &singleMessageReceiver{}
	shutdownGracePeriod := 25 * time.Millisecond

	processor := NewProcessor(receiver, handler.Handle, &ProcessorOptions{
		ReceiveInterval:         durationPtr(time.Hour),
		ShutdownGracePeriod:     &shutdownGracePeriod,
		StartMaxAttempt:         3,
		StartRetryDelayStrategy: &ConstantDelayStrategy{Delay: 0},
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- processor.Start(ctx) }()
	handler.waitStarted(t, errCh)

	cancel()
	start := time.Now()
	err := waitForShutdownResult(t, errCh)
	elapsed := time.Since(start)
	handler.releaseAndWait(t)

	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, receiver.receiveCalls)
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

func assertProcessorStillRunning(t *testing.T, errCh <-chan error) {
	t.Helper()

	select {
	case err := <-errCh:
		t.Fatalf("processor returned before the handler was released: %v", err)
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

func waitForSignal(t *testing.T, signal <-chan struct{}, errCh <-chan error, timeoutMessage string) {
	t.Helper()

	select {
	case <-signal:
	case err := <-errCh:
		t.Fatalf("processor returned early: %v", err)
	case <-time.After(time.Second):
		t.Fatal(timeoutMessage)
	}
}

// shutdownBlockedHandler keeps a processor handler in flight until the test releases it.
type shutdownBlockedHandler struct {
	started chan struct{}
	release chan struct{}
	done    chan struct{}
}

func newShutdownBlockedHandler() *shutdownBlockedHandler {
	return &shutdownBlockedHandler{
		started: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (h *shutdownBlockedHandler) Handle(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
	close(h.started)
	defer close(h.done)
	<-h.release
}

func (h *shutdownBlockedHandler) waitStarted(t *testing.T, errCh <-chan error) {
	t.Helper()
	waitForSignal(t, h.started, errCh, "handler did not start")
}

func (h *shutdownBlockedHandler) releaseAndWait(t *testing.T) {
	t.Helper()

	close(h.release)
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("handler did not return")
	}
}

type shutdownTestSettler struct{}

func (shutdownTestSettler) AbandonMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.AbandonMessageOptions) error {
	return nil
}

func (shutdownTestSettler) CompleteMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.CompleteMessageOptions) error {
	return nil
}

func (shutdownTestSettler) DeadLetterMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeadLetterOptions) error {
	return nil
}

func (shutdownTestSettler) DeferMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeferMessageOptions) error {
	return nil
}

func (shutdownTestSettler) RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error {
	return nil
}

type singleMessageReceiver struct {
	shutdownTestSettler
	receiveCalls int
}

func (r *singleMessageReceiver) ReceiveMessages(ctx context.Context, maxMessages int, options *azservicebus.ReceiveMessagesOptions) ([]*azservicebus.ReceivedMessage, error) {
	r.receiveCalls++
	if r.receiveCalls == 1 {
		return []*azservicebus.ReceivedMessage{{}}, nil
	}
	return nil, errors.New("unexpected retry")
}

type contextErrorAfterFirstReceive struct {
	shutdownTestSettler
	receiveCalls         int
	secondReceiveStarted chan<- struct{}
	closeSecondReceive   sync.Once
}

func (r *contextErrorAfterFirstReceive) ReceiveMessages(ctx context.Context, maxMessages int, options *azservicebus.ReceiveMessagesOptions) ([]*azservicebus.ReceivedMessage, error) {
	r.receiveCalls++
	if r.receiveCalls == 1 {
		return []*azservicebus.ReceivedMessage{{}}, nil
	}

	r.closeSecondReceive.Do(func() {
		close(r.secondReceiveStarted)
	})
	<-ctx.Done()
	return nil, ctx.Err()
}
