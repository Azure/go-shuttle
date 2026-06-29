package shuttle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

const inFlightMessageAbandonTimeout = 10 * time.Second

type inFlightMessages struct {
	mu      sync.RWMutex
	tracked map[*azservicebus.ReceivedMessage]struct{}
}

func newInFlightMessages() *inFlightMessages {
	return &inFlightMessages{
		tracked: make(map[*azservicebus.ReceivedMessage]struct{}),
	}
}

func (m *inFlightMessages) track(message *azservicebus.ReceivedMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tracked[message] = struct{}{}
}

func (m *inFlightMessages) forget(message *azservicebus.ReceivedMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tracked, message)
}

func (m *inFlightMessages) close(ctx context.Context, settler MessageSettler) error {
	messages := m.messages()
	abandonResults := make(chan error, len(messages))

	for _, message := range messages {
		message := message
		go func() {
			abandonResults <- m.abandon(ctx, settler, message)
		}()
	}

	var errs []error
	for range messages {
		select {
		case err := <-abandonResults:
			if err != nil {
				errs = append(errs, err)
			}
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
			errs = append(errs, drainAbandonErrors(abandonResults)...)
			return errors.Join(errs...)
		}
	}
	return errors.Join(errs...)
}

func (m *inFlightMessages) abandon(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) error {
	abandonCtx, cancel := context.WithTimeout(ctx, inFlightMessageAbandonTimeout)
	defer cancel()

	err := settler.AbandonMessage(abandonCtx, message, nil)
	if err != nil {
		return fmt.Errorf("failed to abandon message %s during processor close: %w", message.MessageID, err)
	}
	m.forget(message)
	return nil
}

func drainAbandonErrors(abandonResults <-chan error) []error {
	var errs []error
	for {
		select {
		case err := <-abandonResults:
			if err != nil {
				errs = append(errs, err)
			}
		default:
			return errs
		}
	}
}

func (m *inFlightMessages) messages() []*azservicebus.ReceivedMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	messages := make([]*azservicebus.ReceivedMessage, 0, len(m.tracked))
	for message := range m.tracked {
		messages = append(messages, message)
	}
	return messages
}
