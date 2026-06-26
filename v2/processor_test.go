package shuttle_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"

	"github.com/Azure/go-shuttle/v2"
)

func MyHandler(timePerMessage time.Duration) shuttle.HandlerFunc {
	return func(ctx context.Context, settler shuttle.MessageSettler, message *azservicebus.ReceivedMessage) {
		// logic
		time.Sleep(timePerMessage)
		err := settler.CompleteMessage(ctx, message, nil)
		if err != nil {
			panic(err)
		}
	}
}

func ExampleProcessor() {
	tokenCredential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		panic(err)
	}
	client, err := azservicebus.NewClient("myservicebus.servicebus.windows.net", tokenCredential, nil)
	if err != nil {
		panic(err)
	}
	receiver, err := client.NewReceiverForSubscription("topic-a", "sub-a", nil)
	if err != nil {
		panic(err)
	}
	lockRenewalInterval := 10 * time.Second
	lockRenewalOptions := &shuttle.LockRenewalOptions{Interval: &lockRenewalInterval}
	p := shuttle.NewProcessor(receiver,
		shuttle.NewPanicHandler(nil,
			shuttle.NewRenewLockHandler(lockRenewalOptions,
				MyHandler(0*time.Second))),
		&shuttle.ProcessorOptions{
			MaxConcurrency:  10,
			StartMaxAttempt: 5,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	err = p.Start(ctx)
	if err != nil {
		panic(err)
	}
	cancel()
}

func TestProcessorStart_DefaultsToMaxConcurrency(t *testing.T) {
	a := require.New(t)
	messages := make(chan *azservicebus.ReceivedMessage, 1)
	messages <- &azservicebus.ReceivedMessage{}
	close(messages)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messages,
	}
	processor := shuttle.NewProcessor(rcv, MyHandler(0*time.Second), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := processor.Start(ctx)
	a.ErrorContains(err, "max receive calls exceeded")
	a.Equal(1, len(rcv.ReceiveCalls), "there should be 1 entry in the ReceiveCalls array")
	a.Equal(1, rcv.ReceiveCalls[0], "the processor should have used the default max concurrency of 1")
}

func TestProcessorStart_ContextCanceledAfterStart(t *testing.T) {
	messages := make(chan *azservicebus.ReceivedMessage, 3)
	messages <- &azservicebus.ReceivedMessage{}
	messages <- &azservicebus.ReceivedMessage{}
	messages <- &azservicebus.ReceivedMessage{}
	close(messages)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messages,
		SetupMaxReceiveCalls:  2,
	}
	processor := shuttle.NewProcessor(rcv, MyHandler(0*time.Millisecond),
		&shuttle.ProcessorOptions{
			ReceiveInterval: to.Ptr(1 * time.Second),
		})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error)
	go func() { errCh <- processor.Start(ctx) }()
	cancel()
	g := NewWithT(t)
	g.Eventually(errCh).Should(Receive(MatchError(context.Canceled)))
}

func TestProcessorStart_ShutdownGracePeriodWaitsForInFlightHandler(t *testing.T) {
	handler := newBlockedHandler()
	messages := messagesChannel(1)
	close(messages)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messages,
		SetupMaxReceiveCalls:  10,
	}

	shutdownGracePeriod := 200 * time.Millisecond
	processor := shuttle.NewProcessor(rcv, handler.Handle,
		&shuttle.ProcessorOptions{
			ReceiveInterval:     to.Ptr(time.Hour),
			ShutdownGracePeriod: &shutdownGracePeriod,
		})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- processor.Start(ctx) }()
	handler.waitStarted(t, errCh)

	cancel()
	assertProcessorStillRunning(t, errCh)
	handler.releaseAndWait(t)

	a := require.New(t)
	a.ErrorIs(waitForProcessorError(t, errCh), context.Canceled)
}

func TestProcessorStart_ShutdownGracePeriodWaitsWhenReceiveReturnsContextError(t *testing.T) {
	handler := newBlockedHandler()
	secondReceiveStarted := make(chan struct{})
	rcv := &cancelableSecondReceiveReceiver{
		fakeSettler:          &fakeSettler{},
		secondReceiveStarted: secondReceiveStarted,
	}

	shutdownGracePeriod := 200 * time.Millisecond
	processor := shuttle.NewProcessor(rcv, handler.Handle,
		&shuttle.ProcessorOptions{
			MaxConcurrency:      2,
			ReceiveInterval:     to.Ptr(time.Millisecond),
			ShutdownGracePeriod: &shutdownGracePeriod,
		})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- processor.Start(ctx) }()
	handler.waitStarted(t, errCh)
	waitForReceiveStart(t, secondReceiveStarted, errCh)

	cancel()
	assertProcessorStillRunning(t, errCh)
	handler.releaseAndWait(t)

	a := require.New(t)
	a.ErrorIs(waitForProcessorError(t, errCh), context.Canceled)
}

func TestProcessorStart_ShutdownGracePeriodTimesOutWaitingForInFlightHandler(t *testing.T) {
	handler := newBlockedHandler()
	messages := messagesChannel(1)
	close(messages)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messages,
		SetupMaxReceiveCalls:  10,
	}

	shutdownGracePeriod := 25 * time.Millisecond
	processor := shuttle.NewProcessor(rcv, handler.Handle,
		&shuttle.ProcessorOptions{
			ReceiveInterval:     to.Ptr(time.Hour),
			ShutdownGracePeriod: &shutdownGracePeriod,
		})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- processor.Start(ctx) }()
	handler.waitStarted(t, errCh)

	cancel()
	start := time.Now()
	err := waitForProcessorError(t, errCh)
	elapsed := time.Since(start)

	handler.releaseAndWait(t)

	a := require.New(t)
	a.ErrorIs(err, context.Canceled)
	a.ErrorIs(err, context.DeadlineExceeded)
	a.ErrorContains(err, "timed out waiting")
	a.GreaterOrEqual(elapsed, shutdownGracePeriod)
	a.Less(elapsed, time.Second)
}

func TestProcessorStart_CanceledShutdownTimeoutDoesNotRetry(t *testing.T) {
	handler := newBlockedHandler()
	messages := messagesChannel(1)
	close(messages)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messages,
		SetupMaxReceiveCalls:  10,
	}

	shutdownGracePeriod := 25 * time.Millisecond
	processor := shuttle.NewProcessor(rcv, handler.Handle,
		&shuttle.ProcessorOptions{
			ReceiveInterval:         to.Ptr(time.Hour),
			ShutdownGracePeriod:     &shutdownGracePeriod,
			StartMaxAttempt:         3,
			StartRetryDelayStrategy: &shuttle.ConstantDelayStrategy{Delay: 0},
		})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- processor.Start(ctx) }()
	handler.waitStarted(t, errCh)

	cancel()
	start := time.Now()
	err := waitForProcessorError(t, errCh)
	elapsed := time.Since(start)

	handler.releaseAndWait(t)

	a := require.New(t)
	a.ErrorIs(err, context.Canceled)
	a.ErrorIs(err, context.DeadlineExceeded)
	a.Equal(1, len(rcv.ReceiveCalls))
	a.GreaterOrEqual(elapsed, shutdownGracePeriod)
	a.Less(elapsed, time.Second)
}

func TestProcessorStart_ShutdownGracePeriodDisabledByDefault(t *testing.T) {
	handler := newBlockedHandler()
	messages := messagesChannel(1)
	close(messages)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messages,
		SetupMaxReceiveCalls:  10,
	}

	processor := shuttle.NewProcessor(rcv, handler.Handle,
		&shuttle.ProcessorOptions{
			ReceiveInterval: to.Ptr(time.Hour),
		})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- processor.Start(ctx) }()
	handler.waitStarted(t, errCh)

	cancel()
	a := require.New(t)
	a.ErrorIs(waitForProcessorError(t, errCh), context.Canceled)

	handler.releaseAndWait(t)
}

func TestProcessorStart_CanSetMaxConcurrency(t *testing.T) {
	a := require.New(t)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: make(chan *azservicebus.ReceivedMessage),
	}
	close(rcv.SetupReceivedMessages)
	processor := shuttle.NewProcessor(rcv, MyHandler(0*time.Second), &shuttle.ProcessorOptions{
		MaxConcurrency: 10,
	})
	ctx, cancel := context.WithCancel(context.Background())
	// pre-cancel the context
	cancel()
	err := processor.Start(ctx)
	a.ErrorContains(err, "max receive calls exceeded")
	a.Equal(1, len(rcv.ReceiveCalls), "there should be 1 entry in the ReceiveCalls array")
	a.Equal(10, rcv.ReceiveCalls[0], "the processor should have used max concurrency of 10")
}

func TestProcessorStart_MaxReceiveCountOverridesMaxConcurrency(t *testing.T) {
	a := require.New(t)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupMaxReceiveCalls:  2,
		SetupReceivedMessages: messagesChannel(10),
	}
	close(rcv.SetupReceivedMessages)
	processor := shuttle.NewProcessor(rcv, MyHandler(1*time.Second), &shuttle.ProcessorOptions{
		MaxConcurrency:  10,
		MaxReceiveCount: 4,
		ReceiveInterval: to.Ptr(20 * time.Millisecond),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := processor.Start(ctx)
	a.ErrorContains(err, "max receive calls exceeded")
	a.Equal(2, len(rcv.ReceiveCalls), "there should be 1 entry in the ReceiveCalls array")
	a.Equal(4, rcv.ReceiveCalls[0], "the processor should have used max receive count of 4 initially")
	a.Equal(4, rcv.ReceiveCalls[1], "the processor should have received 4 because it is less than available concurrency")
}

func TestProcessorStart_MaxReceiveCountOverridesMaxConcurrencyReceiveDelta(t *testing.T) {
	a := require.New(t)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupMaxReceiveCalls:  2,
		SetupReceivedMessages: messagesChannel(10),
	}
	close(rcv.SetupReceivedMessages)
	processor := shuttle.NewProcessor(rcv, MyHandler(1*time.Second), &shuttle.ProcessorOptions{
		MaxConcurrency:  10,
		MaxReceiveCount: 6,
		ReceiveInterval: to.Ptr(20 * time.Millisecond),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := processor.Start(ctx)
	a.ErrorContains(err, "max receive calls exceeded")
	a.Equal(2, len(rcv.ReceiveCalls), "there should be 1 entry in the ReceiveCalls array")
	a.Equal(6, rcv.ReceiveCalls[0], "the processor should have used max receive count of 6 initially")
	a.Equal(4, rcv.ReceiveCalls[1], "the processor should have received 4 because of the 6 ongoing messages")
}

func TestProcessorStart_MaxReceiveCountGreaterThanMaxConcurrency(t *testing.T) {
	a := require.New(t)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: make(chan *azservicebus.ReceivedMessage),
	}
	close(rcv.SetupReceivedMessages)
	processor := shuttle.NewProcessor(rcv, MyHandler(0*time.Second), &shuttle.ProcessorOptions{
		MaxConcurrency:  5,
		MaxReceiveCount: 10,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := processor.Start(ctx)
	a.ErrorContains(err, "max receive calls exceeded")
	a.Equal(1, len(rcv.ReceiveCalls), "there should be 1 entry in the ReceiveCalls array")
	a.Equal(5, rcv.ReceiveCalls[0], "the processor should have used max concurrency as max receive count")
}

func TestProcessorStart_DisableMaxReceiveCount(t *testing.T) {
	a := require.New(t)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: make(chan *azservicebus.ReceivedMessage),
	}
	close(rcv.SetupReceivedMessages)
	processor := shuttle.NewProcessor(rcv, MyHandler(0*time.Second), &shuttle.ProcessorOptions{
		MaxConcurrency:  5,
		MaxReceiveCount: -1,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := processor.Start(ctx)
	a.ErrorContains(err, "max receive calls exceeded")
	a.Equal(1, len(rcv.ReceiveCalls), "there should be 1 entry in the ReceiveCalls array")
	a.Equal(5, rcv.ReceiveCalls[0], "the processor should have used max concurrency as max receive count")
}

func TestProcessorStart_Interval(t *testing.T) {
	// with an message processing that takes 10ms and an interval polling every 20 ms,
	// we should call receive exactly 3 times to consume all the messages.
	a := require.New(t)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupMaxReceiveCalls:  3,
		SetupReceivedMessages: messagesChannel(7),
	}
	close(rcv.SetupReceivedMessages)

	processor := shuttle.NewProcessor(rcv, MyHandler(10*time.Millisecond), &shuttle.ProcessorOptions{
		MaxConcurrency:  3,
		ReceiveInterval: to.Ptr(20 * time.Millisecond),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := processor.Start(ctx)
	a.Error(err, "expect to exit with error because we consumed all configured messages")
	a.Equal(3, len(rcv.ReceiveCalls), "there should be 2 entry in the ReceiveCalls array")
	a.Equal(3, rcv.ReceiveCalls[0], "the processor should have used max concurrency of 3")
	a.Equal(3, rcv.ReceiveCalls[1], "the processor should have used max concurrency of 3")
	a.Equal(3, rcv.ReceiveCalls[2], "the processor should have used max concurrency of 3")
}

func TestProcessorStart_ReceiveDeltaConcurrencyOnly(t *testing.T) {
	// with an message processing that takes 10ms and an interval polling every 20 ms,
	// we should call receive exactly 3 times to consume all the messages.
	a := require.New(t)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messagesChannel(2),
		SetupMaxReceiveCalls:  3,
	}
	close(rcv.SetupReceivedMessages)
	processor := shuttle.NewProcessor(rcv, MyHandler(20*time.Millisecond), &shuttle.ProcessorOptions{
		MaxConcurrency:  1,
		ReceiveInterval: to.Ptr(12 * time.Millisecond),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err := processor.Start(ctx)
	a.Error(err, "expect to exit with error because we consumed all configured messages")
	a.Equal(3, len(rcv.ReceiveCalls), "there should be 4 entry in the ReceiveCalls array")
	a.Equal(1, rcv.ReceiveCalls[0], "the processor should have used max concurrency of 1 initially")
	a.Equal(1, rcv.ReceiveCalls[1], "the processor should receive 1 when the previous message is done processing and exit")
	a.Equal(1, rcv.ReceiveCalls[2], "the processor should receive 1 when the previous message is done processing and exit")
}

func TestProcessorStart_ReceiveDelta(t *testing.T) {
	// with an message processing that takes 10ms and an interval polling every 20 ms,
	// we should call receive exactly 2 times to consume all the messages.
	a := require.New(t)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messagesChannel(5),
		SetupMaxReceiveCalls:  2,
	}
	processor := shuttle.NewProcessor(rcv, MyHandler(1*time.Second), &shuttle.ProcessorOptions{
		MaxConcurrency:  10,
		ReceiveInterval: to.Ptr(20 * time.Millisecond),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	done := make(chan struct{})
	var processorError error
	go func() {
		processorError = processor.Start(ctx)
		t.Log("exited processor ", processorError)
		close(done)
	}()
	// ensure the 5 initial messages were processed
	time.Sleep(10 * time.Millisecond)
	enqueueCount(rcv.SetupReceivedMessages, 5)
	close(rcv.SetupReceivedMessages)
	<-done
	a.Error(processorError, "expect to exit with error because we consumed all configured messages")
	a.Equal(2, len(rcv.ReceiveCalls), "should be called 3 times")
	a.Equal(10, rcv.ReceiveCalls[0], "the processor should have used max concurrency of 10 initially")
	a.Equal(5, rcv.ReceiveCalls[1], "the processor should request 5 (delta)")
}

func TestProcessorStart_DefaultsToStartMaxAttempt(t *testing.T) {
	a := require.New(t)
	messages := make(chan *azservicebus.ReceivedMessage, 1)
	close(messages)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messages,
		SetupMaxReceiveCalls:  2,
		SetupReceiveError:     fmt.Errorf("fake receive error"),
	}
	processor := shuttle.NewProcessor(rcv, MyHandler(0*time.Second), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := processor.Start(ctx)
	a.ErrorContains(err, "fake receive error")
	a.Equal(1, len(rcv.ReceiveCalls), "there should be 1 entry in the ReceiveCalls array")
	a.Equal(1, rcv.ReceiveCalls[0], "the processor should have used the default max concurrency of 1")
}

func TestProcessorStart_CanSetStartMaxAttempt(t *testing.T) {
	// with a max start attempt of 3 and a 20ms fixed retry strategy,
	// we should have 3 retries before exiting with an error.
	a := require.New(t)
	messages := make(chan *azservicebus.ReceivedMessage, 1)
	close(messages)
	receiveError := fmt.Errorf("fake receive error")
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messages,
		SetupMaxReceiveCalls:  5,
		SetupReceiveError:     receiveError,
	}

	processor := shuttle.NewProcessor(rcv, MyHandler(0*time.Second), &shuttle.ProcessorOptions{
		MaxConcurrency:          1,
		StartMaxAttempt:         3,
		StartRetryDelayStrategy: &shuttle.ConstantDelayStrategy{Delay: 20 * time.Millisecond},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := processor.Start(ctx)
	// matchError
	a.ErrorIs(err, receiveError)
	a.Equal(3, len(rcv.ReceiveCalls), "there should be 3 connection retries")
	a.Equal(1, rcv.ReceiveCalls[0], "the processor should have used the default max concurrency of 1")
	a.Equal(1, rcv.ReceiveCalls[1], "the processor should have used the default max concurrency of 1")
	a.Equal(1, rcv.ReceiveCalls[2], "the processor should have used the default max concurrency of 1")
}

func TestProcessorStart_ContextCanceledDuringStartRetry(t *testing.T) {
	// with a max start attempt of 5 and a 20ms fixed retry strategy,
	// we should have 2 retries before the context is canceled after 30ms.
	a := require.New(t)
	messages := make(chan *azservicebus.ReceivedMessage, 1)
	close(messages)
	receiveError := fmt.Errorf("fake receive error")
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messages,
		SetupMaxReceiveCalls:  10,
		SetupReceiveError:     receiveError,
	}

	processor := shuttle.NewProcessor(rcv, MyHandler(0*time.Second), &shuttle.ProcessorOptions{
		MaxConcurrency:          1,
		StartMaxAttempt:         5,
		StartRetryDelayStrategy: &shuttle.ConstantDelayStrategy{Delay: 20 * time.Millisecond},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := processor.Start(ctx)
	a.ErrorIs(err, context.DeadlineExceeded)
	a.Equal(2, len(rcv.ReceiveCalls), "there should be 2 connection retries")
	a.Equal(1, rcv.ReceiveCalls[0], "the processor should have used the default max concurrency of 1")
	a.Equal(1, rcv.ReceiveCalls[1], "the processor should have retried the receive call once")
}

func TestProcessorStart_RecoversReceiverPanic(t *testing.T) {
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messagesChannel(1),
		SetupMaxReceiveCalls:  2,
		SetupReceivePanic:     "receive panic!",
	}
	close(rcv.SetupReceivedMessages)
	processor := shuttle.NewProcessor(rcv, MyHandler(0*time.Second), nil)
	ctx := context.Background()
	g := NewWithT(t)
	var err error
	g.Expect(func() { err = processor.Start(ctx) }).To(Not(Panic()))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("panic recovered from processor: receive panic!"))
}

func messagesChannel(messageCount int) chan *azservicebus.ReceivedMessage {
	messages := make(chan *azservicebus.ReceivedMessage, messageCount)
	for i := 0; i < messageCount; i++ {
		messages <- &azservicebus.ReceivedMessage{
			LockedUntil: to.Ptr(time.Now().Add(1 * time.Minute)),
		}
	}
	return messages
}

func enqueueCount(q chan *azservicebus.ReceivedMessage, messageCount int) {
	for i := 0; i < messageCount; i++ {
		q <- &azservicebus.ReceivedMessage{
			LockedUntil: to.Ptr(time.Now().Add(1 * time.Minute)),
		}
	}
}

// blockedHandler keeps a processor handler in flight until the test releases it.
type blockedHandler struct {
	started chan struct{}
	release chan struct{}
	done    chan struct{}
}

func newBlockedHandler() *blockedHandler {
	return &blockedHandler{
		started: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (h *blockedHandler) Handle(ctx context.Context, settler shuttle.MessageSettler, message *azservicebus.ReceivedMessage) {
	close(h.started)
	defer close(h.done)
	<-h.release
}

func (h *blockedHandler) waitStarted(t *testing.T, errCh <-chan error) {
	t.Helper()

	select {
	case <-h.started:
	case err := <-errCh:
		t.Fatalf("processor returned before the handler started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
}

func (h *blockedHandler) releaseAndWait(t *testing.T) {
	t.Helper()

	close(h.release)
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("handler did not return")
	}
}

func waitForReceiveStart(t *testing.T, started <-chan struct{}, errCh <-chan error) {
	t.Helper()

	select {
	case <-started:
	case err := <-errCh:
		t.Fatalf("processor returned before receive started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("receive did not start")
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

func waitForProcessorError(t *testing.T, errCh <-chan error) error {
	t.Helper()

	select {
	case err := <-errCh:
		return err
	case <-time.After(time.Second):
		t.Fatal("processor did not return")
		return nil
	}
}

// cancelableSecondReceiveReceiver returns one message, then blocks in ReceiveMessages until the context is canceled.
type cancelableSecondReceiveReceiver struct {
	*fakeSettler
	receiveCalls         int
	secondReceiveStarted chan<- struct{}
}

func (r *cancelableSecondReceiveReceiver) ReceiveMessages(ctx context.Context, maxMessages int, options *azservicebus.ReceiveMessagesOptions) ([]*azservicebus.ReceivedMessage, error) {
	r.receiveCalls++
	if r.receiveCalls == 1 {
		return []*azservicebus.ReceivedMessage{{}}, nil
	}

	close(r.secondReceiveStarted)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestPanicHandler_WithHandlingFunc(t *testing.T) {
	handler := shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler, message *azservicebus.ReceivedMessage) {
		panic("panic!")
	})
	var recovered any
	options := &shuttle.PanicHandlerOptions{
		OnPanicRecovered: func(ctx context.Context, settler shuttle.MessageSettler, message *azservicebus.ReceivedMessage, rec any) {
			recovered = rec
		},
	}
	p := shuttle.NewPanicHandler(options, handler)
	g := NewWithT(t)
	g.Expect(func() { p.Handle(context.TODO(), nil, nil) }).ToNot(Panic())
	g.Expect(recovered).ToNot(BeNil())
}

func TestNewPanicHandler_DefaultOptions(t *testing.T) {
	handler := shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler, message *azservicebus.ReceivedMessage) {
		panic("panic!")
	})
	p := shuttle.NewPanicHandler(nil, handler)
	g := NewWithT(t)
	g.Expect(func() { p.Handle(context.TODO(), nil, nil) }).ToNot(Panic())
}
