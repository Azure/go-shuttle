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
	settler := &blockingInFlightMessageSettler{
		abandonStarted: abandonStarted,
		releaseAbandon: releaseAbandons,
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
	settler := &contextIgnoringBlockingInFlightMessageSettler{
		abandonStarted: abandonStarted,
		releaseAbandon: releaseAbandon,
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

	release()
	require.Eventually(t, func() bool {
		return len(inFlight.messages()) == 0
	}, 1*time.Second, 10*time.Millisecond)
}

func TestInFlightMessages_CloseRemovesTrackedMessages(t *testing.T) {
	inFlight := newInFlightMessages()
	settler := &inFlightMessageSettler{}
	message := &azservicebus.ReceivedMessage{MessageID: "message"}

	inFlight.track(message)

	require.NoError(t, inFlight.close(context.Background(), settler))
	require.NoError(t, inFlight.close(context.Background(), settler))

	messages := settler.abandonedMessages()
	require.Len(t, messages, 1)
	require.Same(t, message, messages[0])
}

func TestInFlightMessages_CloseReturnsAbandonErrors(t *testing.T) {
	firstErr := errors.New("first abandon failed")
	secondErr := errors.New("second abandon failed")
	inFlight := newInFlightMessages()
	settler := &inFlightMessageSettler{
		abandonErrors: []error{firstErr, secondErr},
	}

	inFlight.track(&azservicebus.ReceivedMessage{MessageID: "first"})
	inFlight.track(&azservicebus.ReceivedMessage{MessageID: "second"})

	err := inFlight.close(context.Background(), settler)

	require.Error(t, err)
	require.ErrorIs(t, err, firstErr)
	require.ErrorIs(t, err, secondErr)
}

func TestInFlightMessages_CloseKeepsMessagesWhenAbandonFails(t *testing.T) {
	abandonErr := errors.New("abandon failed")
	inFlight := newInFlightMessages()
	failingSettler := &inFlightMessageSettler{
		abandonErrors: []error{abandonErr},
	}
	message := &azservicebus.ReceivedMessage{MessageID: "message"}

	inFlight.track(message)

	err := inFlight.close(context.Background(), failingSettler)

	require.ErrorIs(t, err, abandonErr)
	messages := inFlight.messages()
	require.Len(t, messages, 1)
	require.Same(t, message, messages[0])

	successfulSettler := &inFlightMessageSettler{}
	require.NoError(t, inFlight.close(context.Background(), successfulSettler))
	require.Empty(t, inFlight.messages())
	abandonedMessages := successfulSettler.abandonedMessages()
	require.Len(t, abandonedMessages, 1)
	require.Same(t, message, abandonedMessages[0])
}

func TestInFlightMessages_CloseUsesAbandonTimeout(t *testing.T) {
	inFlight := newInFlightMessages()
	settler := &inFlightMessageSettler{}

	inFlight.track(&azservicebus.ReceivedMessage{MessageID: "message"})

	beforeClose := time.Now()
	require.NoError(t, inFlight.close(context.Background(), settler))
	afterClose := time.Now()

	deadlines := settler.abandonDeadlines()
	require.Len(t, deadlines, 1)
	require.True(t, deadlines[0].ok)
	require.True(t, deadlines[0].deadline.After(beforeClose.Add(9*time.Second)))
	require.True(t, deadlines[0].deadline.Before(afterClose.Add(11*time.Second)))
}

type inFlightMessageSettler struct {
	mu             sync.Mutex
	abandoned      []*azservicebus.ReceivedMessage
	deadlines      []abandonDeadline
	abandonErrors  []error
	abandonAttempt int
}

type abandonDeadline struct {
	deadline time.Time
	ok       bool
}

func (s *inFlightMessageSettler) AbandonMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.AbandonMessageOptions) error {
	deadline, ok := ctx.Deadline()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.abandoned = append(s.abandoned, message)
	s.deadlines = append(s.deadlines, abandonDeadline{
		deadline: deadline,
		ok:       ok,
	})
	err := error(nil)
	if s.abandonAttempt < len(s.abandonErrors) {
		err = s.abandonErrors[s.abandonAttempt]
	}
	s.abandonAttempt++
	return err
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

func (s *inFlightMessageSettler) abandonDeadlines() []abandonDeadline {
	s.mu.Lock()
	defer s.mu.Unlock()
	deadlines := make([]abandonDeadline, len(s.deadlines))
	copy(deadlines, s.deadlines)
	return deadlines
}

type blockingInFlightMessageSettler struct {
	abandonStarted chan<- *azservicebus.ReceivedMessage
	releaseAbandon <-chan struct{}
}

func (s *blockingInFlightMessageSettler) AbandonMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.AbandonMessageOptions) error {
	select {
	case s.abandonStarted <- message:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case <-s.releaseAbandon:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *blockingInFlightMessageSettler) CompleteMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.CompleteMessageOptions) error {
	return nil
}

func (s *blockingInFlightMessageSettler) DeadLetterMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeadLetterOptions) error {
	return nil
}

func (s *blockingInFlightMessageSettler) DeferMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeferMessageOptions) error {
	return nil
}

func (s *blockingInFlightMessageSettler) RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error {
	return nil
}

type contextIgnoringBlockingInFlightMessageSettler struct {
	abandonStarted chan<- *azservicebus.ReceivedMessage
	releaseAbandon <-chan struct{}
}

func (s *contextIgnoringBlockingInFlightMessageSettler) AbandonMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.AbandonMessageOptions) error {
	s.abandonStarted <- message
	<-s.releaseAbandon
	return nil
}

func (s *contextIgnoringBlockingInFlightMessageSettler) CompleteMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.CompleteMessageOptions) error {
	return nil
}

func (s *contextIgnoringBlockingInFlightMessageSettler) DeadLetterMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeadLetterOptions) error {
	return nil
}

func (s *contextIgnoringBlockingInFlightMessageSettler) DeferMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeferMessageOptions) error {
	return nil
}

func (s *contextIgnoringBlockingInFlightMessageSettler) RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error {
	return nil
}
