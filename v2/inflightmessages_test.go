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

func TestInFlightMessages_CloseAbandonsTrackedMessages(t *testing.T) {
	inFlight := newInFlightMessages()
	settler := &inFlightMessageSettler{}
	trackedMessage := &azservicebus.ReceivedMessage{MessageID: "tracked"}
	forgottenMessage := &azservicebus.ReceivedMessage{MessageID: "forgotten"}

	inFlight.track(trackedMessage)
	inFlight.track(forgottenMessage)
	inFlight.forget(forgottenMessage)

	require.NoError(t, inFlight.close(context.Background(), settler))
	require.Empty(t, inFlight.messages())
	require.NoError(t, inFlight.close(context.Background(), settler))

	messages := settler.abandonedMessages()
	require.Len(t, messages, 1)
	require.Same(t, trackedMessage, messages[0])
}

func TestInFlightMessages_CloseAbandonsTrackedMessagesConcurrently(t *testing.T) {
	inFlight := newInFlightMessages()
	firstMessage := &azservicebus.ReceivedMessage{MessageID: "first"}
	secondMessage := &azservicebus.ReceivedMessage{MessageID: "second"}
	abandonStarted := make(chan *azservicebus.ReceivedMessage, 2)
	releaseAbandons := make(chan struct{})
	var releaseOnce sync.Once
	settler := &inFlightMessageSettler{
		abandon: func(ctx context.Context, message *azservicebus.ReceivedMessage) error {
			abandonStarted <- message
			<-releaseAbandons
			return nil
		},
	}

	release := func() {
		releaseOnce.Do(func() {
			close(releaseAbandons)
		})
	}
	t.Cleanup(release)

	inFlight.track(firstMessage)
	inFlight.track(secondMessage)

	closeErr := make(chan error, 1)
	go func() {
		closeErr <- inFlight.close(context.Background(), settler)
	}()

	abandoned := map[*azservicebus.ReceivedMessage]struct{}{}
	for len(abandoned) < 2 {
		select {
		case message := <-abandonStarted:
			abandoned[message] = struct{}{}
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for concurrent abandon attempts")
		}
	}

	select {
	case err := <-closeErr:
		t.Fatalf("close returned before abandon attempts were released: %v", err)
	default:
	}

	release()
	require.NoError(t, <-closeErr)
	require.Contains(t, abandoned, firstMessage)
	require.Contains(t, abandoned, secondMessage)
}

func TestInFlightMessages_CloseReturnsWhenContextIsCanceled(t *testing.T) {
	inFlight := newInFlightMessages()
	abandonStarted := make(chan *azservicebus.ReceivedMessage, 1)
	releaseAbandon := make(chan struct{})
	var releaseOnce sync.Once
	settler := &inFlightMessageSettler{
		abandon: func(ctx context.Context, message *azservicebus.ReceivedMessage) error {
			abandonStarted <- message
			<-releaseAbandon
			return nil
		},
	}

	release := func() {
		releaseOnce.Do(func() {
			close(releaseAbandon)
		})
	}
	t.Cleanup(release)

	inFlight.track(&azservicebus.ReceivedMessage{MessageID: "blocked"})

	ctx, cancel := context.WithCancel(context.Background())
	closeErr := make(chan error, 1)
	go func() {
		closeErr <- inFlight.close(ctx, settler)
	}()

	select {
	case <-abandonStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for abandon attempt to start")
	}

	cancel()

	select {
	case err := <-closeErr:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for close to return after context cancellation")
	}

	abandonedMessages := settler.abandonedMessages()
	require.Len(t, abandonedMessages, 1)
	require.Equal(t, "blocked", abandonedMessages[0].MessageID)

	release()
	require.Eventually(t, func() bool {
		return len(inFlight.messages()) == 0
	}, 1*time.Second, 10*time.Millisecond)
}

func TestInFlightMessages_CloseReturnsWhenContextDeadlineExpires(t *testing.T) {
	inFlight := newInFlightMessages()
	abandonStarted := make(chan *azservicebus.ReceivedMessage, 1)
	releaseAbandon := make(chan struct{})
	var releaseOnce sync.Once
	settler := &inFlightMessageSettler{
		abandon: func(ctx context.Context, message *azservicebus.ReceivedMessage) error {
			abandonStarted <- message
			<-releaseAbandon
			return nil
		},
	}

	release := func() {
		releaseOnce.Do(func() {
			close(releaseAbandon)
		})
	}
	t.Cleanup(release)

	inFlight.track(&azservicebus.ReceivedMessage{MessageID: "deadline"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	closeErr := make(chan error, 1)
	go func() {
		closeErr <- inFlight.close(ctx, settler)
	}()

	select {
	case <-abandonStarted:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for abandon attempt to start")
	}

	select {
	case err := <-closeErr:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for close to return after context deadline")
	}

	abandonedMessages := settler.abandonedMessages()
	require.Len(t, abandonedMessages, 1)
	require.Equal(t, "deadline", abandonedMessages[0].MessageID)

	release()
	require.Eventually(t, func() bool {
		return len(inFlight.messages()) == 0
	}, 1*time.Second, 10*time.Millisecond)
}

func TestInFlightMessages_CloseReturnsAbandonErrorsAndKeepsMessages(t *testing.T) {
	firstErr := errors.New("first abandon failed")
	secondErr := errors.New("second abandon failed")
	inFlight := newInFlightMessages()
	firstMessage := &azservicebus.ReceivedMessage{MessageID: "first"}
	secondMessage := &azservicebus.ReceivedMessage{MessageID: "second"}
	abandonErrors := make(chan error, 2)
	abandonErrors <- firstErr
	abandonErrors <- secondErr
	settler := &inFlightMessageSettler{
		abandon: func(ctx context.Context, message *azservicebus.ReceivedMessage) error {
			return <-abandonErrors
		},
	}

	inFlight.track(firstMessage)
	inFlight.track(secondMessage)

	err := inFlight.close(context.Background(), settler)

	require.Error(t, err)
	require.ErrorIs(t, err, firstErr)
	require.ErrorIs(t, err, secondErr)
	require.ElementsMatch(t, []*azservicebus.ReceivedMessage{firstMessage, secondMessage}, inFlight.messages())

	successfulSettler := &inFlightMessageSettler{}
	require.NoError(t, inFlight.close(context.Background(), successfulSettler))
	require.Empty(t, inFlight.messages())
	abandonedMessages := successfulSettler.abandonedMessages()
	require.ElementsMatch(t, []*azservicebus.ReceivedMessage{firstMessage, secondMessage}, abandonedMessages)
}

type inFlightMessageSettler struct {
	mu        sync.Mutex
	abandoned []*azservicebus.ReceivedMessage
	abandon   func(context.Context, *azservicebus.ReceivedMessage) error
}

func (s *inFlightMessageSettler) AbandonMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.AbandonMessageOptions) error {
	s.mu.Lock()
	s.abandoned = append(s.abandoned, message)
	s.mu.Unlock()

	if s.abandon != nil {
		return s.abandon(ctx, message)
	}
	return nil
}

func (s *inFlightMessageSettler) CompleteMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.CompleteMessageOptions) error {
	return nil
}

func (s *inFlightMessageSettler) DeadLetterMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeadLetterOptions) error {
	return nil
}

func (s *inFlightMessageSettler) DeferMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeferMessageOptions) error {
	return nil
}

func (s *inFlightMessageSettler) RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error {
	return nil
}

func (s *inFlightMessageSettler) abandonedMessages() []*azservicebus.ReceivedMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := make([]*azservicebus.ReceivedMessage, len(s.abandoned))
	copy(messages, s.abandoned)
	return messages
}
