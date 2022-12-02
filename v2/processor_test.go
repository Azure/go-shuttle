package v2_test

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"

	"github.com/stretchr/testify/require"

	v2 "github.com/Azure/go-shuttle/v2"
)

func MyHandler(timePerMessage time.Duration) v2.HandlerFunc {
	return func(ctx context.Context, settler v2.MessageSettler, message *azservicebus.ReceivedMessage) {
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
	p := v2.NewProcessor(receiver,
		v2.NewPanicHandler(
			v2.NewRenewLockHandler(receiver, &lockRenewalInterval,
				MyHandler(0*time.Second))), &v2.ProcessorOptions{MaxConcurrency: 10})

	ctx, cancel := context.WithCancel(context.Background())
	err = p.Start(ctx)
	if err != nil {
		panic(err)
	}
	cancel()
}

func TestNewProcessor_DefaultsToMaxConcurrency1(t *testing.T) {
	a := require.New(t)
	messages := make(chan *azservicebus.ReceivedMessage, 1)
	messages <- &azservicebus.ReceivedMessage{}
	close(messages)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messages,
	}
	processor := v2.NewProcessor(rcv, MyHandler(0*time.Second), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := processor.Start(ctx)
	a.EqualError(err, "max receive calls exceeded")
	a.Equal(1, len(rcv.ReceiveCalls), "there should be 1 entry in the ReceiveCalls array")
	a.Equal(1, rcv.ReceiveCalls[0], "the processor should have used the default max concurrency of 1")
}

func TestNewProcessor_CanSetMaxConcurrency(t *testing.T) {
	a := require.New(t)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: make(chan *azservicebus.ReceivedMessage, 0),
	}
	close(rcv.SetupReceivedMessages)
	processor := v2.NewProcessor(rcv, MyHandler(0*time.Second), &v2.ProcessorOptions{
		MaxConcurrency: 10,
	})
	ctx, cancel := context.WithCancel(context.Background())
	// pre-cancel the context
	cancel()
	err := processor.Start(ctx)
	a.EqualError(err, "max receive calls exceeded")
	a.Equal(1, len(rcv.ReceiveCalls), "there should be 1 entry in the ReceiveCalls array")
	a.Equal(10, rcv.ReceiveCalls[0], "the processor should have used max concurrency of 10")
}

func TestNewProcessor_Interval(t *testing.T) {
	// with an message processing that takes 10ms and an interval polling every 20 ms,
	// we should call receive exactly 3 times to consume all the messages.
	a := require.New(t)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupMaxReceiveCalls:  3,
		SetupReceivedMessages: messagesChannel(7),
	}
	close(rcv.SetupReceivedMessages)

	processor := v2.NewProcessor(rcv, MyHandler(10*time.Millisecond), &v2.ProcessorOptions{
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

func TestNewProcessor_ReceiveDeltaConcurrencyOnly(t *testing.T) {
	// with an message processing that takes 10ms and an interval polling every 20 ms,
	// we should call receive exactly 3 times to consume all the messages.
	a := require.New(t)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messagesChannel(2),
		SetupMaxReceiveCalls:  3,
	}
	close(rcv.SetupReceivedMessages)
	processor := v2.NewProcessor(rcv, MyHandler(20*time.Millisecond), &v2.ProcessorOptions{
		MaxConcurrency:  1,
		ReceiveInterval: to.Ptr(12 * time.Millisecond),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err := processor.Start(ctx)
	a.Error(err, "expect to exit with error because we consumed all configured messages")
	a.Equal(3, len(rcv.ReceiveCalls), "there should be 4 entry in the ReceiveCalls array")
	a.Equal(1, rcv.ReceiveCalls[0], "the processor should have used max concurrency of 1 initially")
	a.Equal(0, rcv.ReceiveCalls[1], "the processor has no go-routine available")
	a.Equal(1, rcv.ReceiveCalls[2], "the processor should receive 1 when the previous message is done processing and exit")
}

func TestNewProcessor_ReceiveDelta(t *testing.T) {
	// with an message processing that takes 10ms and an interval polling every 20 ms,
	// we should call receive exactly 3 times to consume all the messages.
	a := require.New(t)
	rcv := &fakeReceiver{
		fakeSettler:           &fakeSettler{},
		SetupReceivedMessages: messagesChannel(5),
		SetupMaxReceiveCalls:  3,
	}
	processor := v2.NewProcessor(rcv, MyHandler(1*time.Second), &v2.ProcessorOptions{
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
	a.Equal(3, len(rcv.ReceiveCalls), "should be called 3 times")
	a.Equal(10, rcv.ReceiveCalls[0], "the processor should have used max concurrency of 10 initially")
	a.Equal(5, rcv.ReceiveCalls[1], "the processor should request 5 (delta)")
	a.Equal(0, rcv.ReceiveCalls[2], "the processor should request 0 (delta)")
}
