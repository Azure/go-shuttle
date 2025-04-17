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

func ExampleProcessor_multiProcessor() {
	tokenCredential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		panic(err)
	}
	client, err := azservicebus.NewClient("myservicebus-1.servicebus.windows.net", tokenCredential, nil)
	if err != nil {
		panic(err)
	}
	receiver1, err := client.NewReceiverForSubscription("topic-a", "sub-a", nil)
	if err != nil {
		panic(err)
	}
	client, err = azservicebus.NewClient("myservicebus-2.servicebus.windows.net", tokenCredential, nil)
	if err != nil {
		panic(err)
	}
	receiver2, err := client.NewReceiverForSubscription("topic-a", "sub-a", nil)
	if err != nil {
		panic(err)
	}
	receivers := []*shuttle.ReceiverEx{
		shuttle.NewReceiverEx("receiver1", receiver1),
		shuttle.NewReceiverEx("receiver2", receiver2),
	}
	lockRenewalInterval := 10 * time.Second
	lockRenewalOptions := &shuttle.LockRenewalOptions{Interval: &lockRenewalInterval}
	p := shuttle.NewMultiProcessor(receivers,
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

func TestProcessorStart_TwoReceivers(t *testing.T) {
	// with an message processing that takes 10ms and an interval polling every 20 ms,
	// we should call receive exactly 3 times to consume all the messages.
	a := require.New(t)
	rcv1 := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupMaxReceiveCalls:  3,
		SetupReceivedMessages: messagesChannel(7),
	}
	close(rcv1.SetupReceivedMessages)

	rcv2 := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupMaxReceiveCalls:  3,
		SetupReceivedMessages: messagesChannel(7),
	}
	close(rcv2.SetupReceivedMessages)

	rcvs := []*shuttle.ReceiverEx{
		shuttle.NewReceiverEx("testReceiver1", rcv1),
		shuttle.NewReceiverEx("testReceiver2", rcv2),
	}

	processor := shuttle.NewMultiProcessor(rcvs, MyHandler(10*time.Millisecond), &shuttle.ProcessorOptions{
		MaxConcurrency:  3,
		ReceiveInterval: to.Ptr(20 * time.Millisecond),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := processor.Start(ctx)
	a.Error(err, "expect to exit with error because we consumed all configured messages")
	for i, rcv := range []*fakeReceiver{rcv1, rcv2} {
		a.ErrorContains(err, fmt.Sprintf("processor testReceiver%d failed to receive messages: max receive calls exceeded", i+1))
		a.Equal(3, len(rcv.ReceiveCalls), "there should be 3 entry in the ReceiveCalls array")
		a.Equal(3, rcv.ReceiveCalls[0], "the processor should have used max concurrency of 3")
	}
}

func TestProcessorStart_TwoReceiversOneErrorOneSuccess(t *testing.T) {
	// with an message processing that takes 10ms and an interval polling every 20 ms,
	// we should call receive exactly 3 times to consume all the messages.
	a := require.New(t)
	rcv1 := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupMaxReceiveCalls:  3,
		SetupReceivedMessages: messagesChannel(7),
	}
	close(rcv1.SetupReceivedMessages)

	rcv2 := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupMaxReceiveCalls:  3,
		SetupReceivedMessages: messagesChannel(7),
		SetupReceiveError:     fmt.Errorf("fake receive error"),
	}
	close(rcv2.SetupReceivedMessages)

	rcvs := []*shuttle.ReceiverEx{
		shuttle.NewReceiverEx("testReceiver1", rcv1),
		shuttle.NewReceiverEx("testReceiver2", rcv2),
	}

	processor := shuttle.NewMultiProcessor(rcvs, MyHandler(10*time.Millisecond), &shuttle.ProcessorOptions{
		MaxConcurrency:  3,
		ReceiveInterval: to.Ptr(20 * time.Millisecond),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := processor.Start(ctx)
	a.Error(err, "expect to exit with error because we consumed all configured messages and one receiver failed to receive messages")
	a.ErrorContains(err, "processor testReceiver1 failed to receive messages: max receive calls exceeded")
	a.ErrorContains(err, "processor testReceiver2 failed to receive messages: fake receive error")
	a.Equal(3, len(rcv1.ReceiveCalls), "there should be 3 entry in the ReceiveCalls array")
	a.Equal(3, rcv1.ReceiveCalls[0], "the processor should have used max concurrency of 3")
	a.Equal(1, len(rcv2.ReceiveCalls), "there should be 1 entry in the ReceiveCalls array")
	a.Equal(3, rcv2.ReceiveCalls[0], "the processor should have used max concurrency of 3")
}

func TestProcessorStart_TwoReceiversWithStartRetry(t *testing.T) {
	// with an message processing that takes 10ms and an interval polling every 20 ms,
	// we should call receive exactly 3 times to consume all the messages.
	a := require.New(t)
	rcv1 := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupMaxReceiveCalls:  3,
		SetupReceivedMessages: messagesChannel(7),
	}
	close(rcv1.SetupReceivedMessages)

	rcv2 := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupMaxReceiveCalls:  3,
		SetupReceivedMessages: messagesChannel(7),
		SetupReceiveError:     fmt.Errorf("fake receive error"),
	}
	close(rcv2.SetupReceivedMessages)

	rcvs := []*shuttle.ReceiverEx{
		shuttle.NewReceiverEx("testReceiver1", rcv1),
		shuttle.NewReceiverEx("testReceiver2", rcv2),
	}

	processor := shuttle.NewMultiProcessor(rcvs, MyHandler(10*time.Millisecond), &shuttle.ProcessorOptions{
		MaxConcurrency:          3,
		ReceiveInterval:         to.Ptr(20 * time.Millisecond),
		StartMaxAttempt:         2,
		StartRetryDelayStrategy: &shuttle.ConstantDelayStrategy{Delay: 10 * time.Millisecond},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := processor.Start(ctx)
	a.Error(err, "expect to exit with error because we consumed all configured messages and one receiver failed to receive messages")
	a.ErrorContains(err, "processor testReceiver1 failed to receive messages: max receive calls exceeded")
	a.ErrorContains(err, "processor testReceiver2 failed to receive messages: fake receive error")
	a.Equal(4, len(rcv1.ReceiveCalls), "there should be 4 entry in the ReceiveCalls array (3 receives and 1 retry)")
	a.Equal(3, rcv1.ReceiveCalls[0], "the processor should have used max concurrency of 3")
	a.Equal(2, len(rcv2.ReceiveCalls), "there should be 1 entry in the ReceiveCalls array (1 retry)")
	a.Equal(3, rcv2.ReceiveCalls[0], "the processor should have used max concurrency of 3")
}

func TestProcessorStart_MultiReceivers(t *testing.T) {
	a := require.New(t)
	rcvs := make([]*shuttle.ReceiverEx, 0)
	fakeReceivers := make([]*fakeReceiver, 0)
	expectedReceiveCalls := []int{1, 1, 1, 2, 2}
	for i := 0; i < 5; i++ {
		fakeReceiver := &fakeReceiver{
			fakeSettler:           &fakeSettler{},
			SetupMaxReceiveCalls:  expectedReceiveCalls[i],
			SetupReceivedMessages: messagesChannel(i),
		}
		rcvs = append(rcvs, shuttle.NewReceiverEx(fmt.Sprintf("testReceiver%d", i), fakeReceiver))
		fakeReceivers = append(fakeReceivers, fakeReceiver)
		close(fakeReceiver.SetupReceivedMessages)
	}

	processor := shuttle.NewMultiProcessor(rcvs, MyHandler(10*time.Millisecond), &shuttle.ProcessorOptions{
		MaxConcurrency:  3,
		ReceiveInterval: to.Ptr(20 * time.Millisecond),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := processor.Start(ctx)
	a.Error(err, "expect to exit with error because we consumed all configured messages")

	for i, fakeReceiver := range fakeReceivers {
		a.ErrorContains(err, fmt.Sprintf("processor testReceiver%d failed to receive messages: max receive calls exceeded", i))
		a.Equal(len(fakeReceiver.ReceiveCalls), expectedReceiveCalls[i], "receiver %d should have received %d messages", i, expectedReceiveCalls[i])
		a.Equal(fakeReceiver.ReceiveCalls[0], 3, "receiver %d should have used max concurrency of 3", i)
	}
}

func TestProcessorStart_SinglePanic(t *testing.T) {
	rcv1 := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messagesChannel(1),
		SetupMaxReceiveCalls:  2,
		SetupReceivePanic:     "receive panic!",
	}
	close(rcv1.SetupReceivedMessages)
	rcv2 := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messagesChannel(1),
		SetupMaxReceiveCalls:  2,
	}
	close(rcv2.SetupReceivedMessages)
	rcvs := []*shuttle.ReceiverEx{
		shuttle.NewReceiverEx("testReceiver1", rcv1),
		shuttle.NewReceiverEx("testReceiver2", rcv2),
	}
	processor := shuttle.NewMultiProcessor(rcvs, MyHandler(0*time.Second), nil)
	ctx := context.Background()
	// expect Start() function to not panic
	g := NewWithT(t)
	var err error
	g.Expect(func() { err = processor.Start(ctx) }).To(Not(Panic()))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("panic recovered from processor testReceiver1: receive panic!"))
	g.Expect(err.Error()).To(ContainSubstring("processor testReceiver2 failed to receive messages: max receive calls exceeded"))
}

func TestProcessorStart_TwoPanics(t *testing.T) {
	rcv1 := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messagesChannel(1),
		SetupMaxReceiveCalls:  2,
		SetupReceivePanic:     "receive panic!",
	}
	close(rcv1.SetupReceivedMessages)
	rcv2 := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messagesChannel(1),
		SetupMaxReceiveCalls:  2,
		SetupReceivePanic:     "receive panic!",
	}
	close(rcv2.SetupReceivedMessages)
	rcvs := []*shuttle.ReceiverEx{
		shuttle.NewReceiverEx("testReceiver1", rcv1),
		shuttle.NewReceiverEx("testReceiver2", rcv2),
	}
	processor := shuttle.NewMultiProcessor(rcvs, MyHandler(0*time.Second), nil)
	ctx := context.Background()
	// expect Start() function to not panic
	g := NewWithT(t)
	var err error
	g.Expect(func() { err = processor.Start(ctx) }).To(Not(Panic()))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("panic recovered from processor testReceiver1: receive panic!"))
	g.Expect(err.Error()).To(ContainSubstring("panic recovered from processor testReceiver2: receive panic!"))
}

func TestProcessorStart_MultiProcessorWithNewRenewLockHandler(t *testing.T) {
	rcv1 := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messagesChannel(1),
		SetupMaxReceiveCalls:  2,
	}
	close(rcv1.SetupReceivedMessages)
	rcv2 := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messagesChannel(1),
		SetupMaxReceiveCalls:  2,
	}
	close(rcv2.SetupReceivedMessages)
	rcvs := []*shuttle.ReceiverEx{
		shuttle.NewReceiverEx("testReceiver1", rcv1),
		shuttle.NewReceiverEx("testReceiver2", rcv2),
	}
	lockRenewalInterval := 50 * time.Millisecond
	lockRenewalOptions := &shuttle.LockRenewalOptions{Interval: &lockRenewalInterval}
	processor := shuttle.NewMultiProcessor(rcvs,
		shuttle.NewRenewLockHandler(lockRenewalOptions, MyHandler(150*time.Millisecond)),
		&shuttle.ProcessorOptions{
			MaxConcurrency: 2,
		})
	ctx, cancel := context.WithTimeout(context.TODO(), 120*time.Millisecond)
	defer cancel()
	err := processor.Start(ctx)
	g := NewWithT(t)
	g.Expect(err).To(HaveOccurred())
	g.Eventually(
		func(g Gomega) { g.Expect(rcv1.RenewCalled.Load()).To(Equal(int32(2))) },
		130*time.Millisecond,
		20*time.Millisecond).Should(Succeed())
	g.Eventually(
		func(g Gomega) { g.Expect(rcv2.RenewCalled.Load()).To(Equal(int32(2))) },
		130*time.Millisecond,
		20*time.Millisecond).Should(Succeed())
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
