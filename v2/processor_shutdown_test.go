package shuttle_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/stretchr/testify/require"

	"github.com/Azure/go-shuttle/v2"
)

func TestProcessorStart_ShutdownGracePeriodDisabledReturnsBeforeHandlerFinishes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		gracePeriod *time.Duration
	}{
		{name: "unset"},
		{name: "zero", gracePeriod: to.Ptr(time.Duration(0))},
		{name: "negative", gracePeriod: to.Ptr(-time.Second)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := newShutdownBlockedHandler()
			receiver := &singleMessageReceiver{fakeSettler: &fakeSettler{}}
			processor := shuttle.NewProcessor(receiver, handler.Handle, &shuttle.ProcessorOptions{
				ReceiveInterval:     to.Ptr(time.Hour),
				ShutdownGracePeriod: tc.gracePeriod,
			})

			ctx, cancel := context.WithCancel(context.Background())
			errCh := startProcessor(ctx, processor)
			handler.waitStarted(t, errCh)
			defer handler.releaseAndWait(t)

			cancel()
			err := waitForProcessorResult(t, errCh)

			require.ErrorIs(t, err, context.Canceled)
			handler.assertStillRunning(t)
		})
	}
}

func TestProcessorStart_ShutdownGracePeriodWaitsForHandler(t *testing.T) {
	handler := newShutdownBlockedHandler()
	secondReceiveStarted := make(chan struct{})
	receiver := &contextErrorAfterFirstReceive{
		fakeSettler:          &fakeSettler{},
		secondReceiveStarted: secondReceiveStarted,
	}
	shutdownGracePeriod := 500 * time.Millisecond

	processor := shuttle.NewProcessor(receiver, handler.Handle, &shuttle.ProcessorOptions{
		MaxConcurrency:      2,
		ReceiveInterval:     to.Ptr(time.Millisecond),
		ShutdownGracePeriod: &shutdownGracePeriod,
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := startProcessor(ctx, processor)
	handler.waitStarted(t, errCh)
	defer handler.releaseAndWait(t)
	waitForSignal(t, secondReceiveStarted, errCh, "second receive did not start")

	cancel()
	assertProcessorStillRunning(t, errCh)
	handler.releaseAndWait(t)

	require.ErrorIs(t, waitForProcessorResult(t, errCh), context.Canceled)
}

func TestProcessorStart_ShutdownGracePeriodTimesOutAndDoesNotRetry(t *testing.T) {
	handler := newShutdownBlockedHandler()
	receiver := &singleMessageReceiver{fakeSettler: &fakeSettler{}}
	shutdownGracePeriod := 150 * time.Millisecond

	processor := shuttle.NewProcessor(receiver, handler.Handle, &shuttle.ProcessorOptions{
		ReceiveInterval:         to.Ptr(time.Hour),
		ShutdownGracePeriod:     &shutdownGracePeriod,
		StartMaxAttempt:         3,
		StartRetryDelayStrategy: &shuttle.ConstantDelayStrategy{Delay: 0},
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := startProcessor(ctx, processor)
	handler.waitStarted(t, errCh)
	defer handler.releaseAndWait(t)

	cancel()
	start := time.Now()
	err := waitForProcessorResult(t, errCh)
	elapsed := time.Since(start)
	handler.releaseAndWait(t)

	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, receiver.receiveCalls)
	require.GreaterOrEqual(t, elapsed, shutdownGracePeriod)
	require.Less(t, elapsed, 2*time.Second)
}

func startProcessor(ctx context.Context, processor *shuttle.Processor) <-chan error {
	errCh := make(chan error, 1)
	go func() { errCh <- processor.Start(ctx) }()
	return errCh
}

func assertProcessorStillRunning(t *testing.T, errCh <-chan error) {
	t.Helper()

	select {
	case err := <-errCh:
		t.Fatalf("processor returned before the handler was released: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}

func waitForProcessorResult(t *testing.T, errCh <-chan error) error {
	t.Helper()

	select {
	case err := <-errCh:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("processor did not return")
		return nil
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, errCh <-chan error, timeoutMessage string) {
	t.Helper()

	select {
	case <-signal:
	case err := <-errCh:
		t.Fatalf("processor returned early: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal(timeoutMessage)
	}
}

type shutdownBlockedHandler struct {
	started     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	done        chan struct{}
}

func newShutdownBlockedHandler() *shutdownBlockedHandler {
	return &shutdownBlockedHandler{
		started: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (h *shutdownBlockedHandler) Handle(_ context.Context, _ shuttle.MessageSettler, _ *azservicebus.ReceivedMessage) {
	close(h.started)
	defer close(h.done)
	<-h.release
}

func (h *shutdownBlockedHandler) waitStarted(t *testing.T, errCh <-chan error) {
	t.Helper()
	waitForSignal(t, h.started, errCh, "handler did not start")
}

func (h *shutdownBlockedHandler) assertStillRunning(t *testing.T) {
	t.Helper()

	select {
	case <-h.done:
		t.Fatal("handler returned before it was released")
	default:
	}
}

func (h *shutdownBlockedHandler) releaseAndWait(t *testing.T) {
	t.Helper()

	h.releaseOnce.Do(func() {
		close(h.release)
	})
	select {
	case <-h.done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return")
	}
}

type singleMessageReceiver struct {
	*fakeSettler
	receiveCalls int
}

func (r *singleMessageReceiver) ReceiveMessages(_ context.Context, _ int, _ *azservicebus.ReceiveMessagesOptions) ([]*azservicebus.ReceivedMessage, error) {
	r.receiveCalls++
	if r.receiveCalls == 1 {
		return []*azservicebus.ReceivedMessage{{}}, nil
	}
	return nil, errors.New("unexpected retry")
}

type contextErrorAfterFirstReceive struct {
	*fakeSettler
	receiveCalls         int
	secondReceiveStarted chan<- struct{}
	closeSecondReceive   sync.Once
}

func (r *contextErrorAfterFirstReceive) ReceiveMessages(ctx context.Context, _ int, _ *azservicebus.ReceiveMessagesOptions) ([]*azservicebus.ReceivedMessage, error) {
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
