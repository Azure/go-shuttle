package shuttle_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"

	v2 "github.com/Azure/go-shuttle/v2"
)

var _ v2.MessageSettler = &fakeSettler{}

type fakeSettler struct {
	AbandonCalled    atomic.Int32
	CompleteCalled   atomic.Int32
	DeadLetterCalled atomic.Int32
	DeferCalled      atomic.Int32
	RenewCalled      atomic.Int32
	SetupAbandonErr  error

	mu                sync.Mutex
	AbandonedMessages []*azservicebus.ReceivedMessage
}

func (f *fakeSettler) AbandonMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.AbandonMessageOptions) error {
	f.AbandonCalled.Add(1)
	f.mu.Lock()
	f.AbandonedMessages = append(f.AbandonedMessages, message)
	f.mu.Unlock()
	return f.SetupAbandonErr
}

func (f *fakeSettler) CompleteMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.CompleteMessageOptions) error {
	f.CompleteCalled.Add(1)
	return nil
}

func (f *fakeSettler) DeadLetterMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeadLetterOptions) error {
	f.DeadLetterCalled.Add(1)
	return nil
}

func (f *fakeSettler) DeferMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeferMessageOptions) error {
	f.DeferCalled.Add(1)
	return nil
}

func (f *fakeSettler) RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error {
	f.RenewCalled.Add(1)
	return nil
}

func (f *fakeSettler) abandonedMessages() []*azservicebus.ReceivedMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	messages := make([]*azservicebus.ReceivedMessage, len(f.AbandonedMessages))
	copy(messages, f.AbandonedMessages)
	return messages
}

type fakeReceiver struct {
	// outcomes to verify
	ReceiveCalls []int // array of maxMessage value passed to receive calls in the lifetime of the fake receiver

	// configure fake
	SetupReceiveError     error
	SetupReceivedMessages chan *azservicebus.ReceivedMessage
	*fakeSettler
	SetupMaxReceiveCalls int
	SetupReceivePanic    string
	SetupReceiveStarted  chan struct{}
}

func (f *fakeReceiver) ReceiveMessages(ctx context.Context, maxMessages int, _ *azservicebus.ReceiveMessagesOptions) ([]*azservicebus.ReceivedMessage, error) {
	f.ReceiveCalls = append(f.ReceiveCalls, maxMessages)
	if f.SetupReceiveStarted != nil {
		select {
		case f.SetupReceiveStarted <- struct{}{}:
		default:
		}
	}
	if maxMessages == 0 && len(f.SetupReceivedMessages) > 0 {
		return nil, nil
	}
	var result []*azservicebus.ReceivedMessage
	for len(result) < maxMessages {
		select {
		case msg, ok := <-f.SetupReceivedMessages:
			if !ok {
				return f.receiveResult(result)
			}
			result = append(result, msg)
			if len(f.SetupReceivedMessages) == 0 {
				return f.receiveResult(result)
			}
		case <-ctx.Done():
			return result, ctx.Err()
		}
	}

	return f.receiveResult(result)
}

func (f *fakeReceiver) receiveResult(result []*azservicebus.ReceivedMessage) ([]*azservicebus.ReceivedMessage, error) {
	if f.SetupReceivePanic != "" {
		panic(f.SetupReceivePanic)
	}

	// return an error if we request more messages than there are available.
	if len(f.ReceiveCalls) >= f.SetupMaxReceiveCalls {
		return result, fmt.Errorf("max receive calls exceeded")
	}

	return result, f.SetupReceiveError
}
