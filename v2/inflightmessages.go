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
	var errs []error
	for _, message := range m.messages() {
		abandonCtx, cancel := context.WithTimeout(ctx, inFlightMessageAbandonTimeout)
		err := settler.AbandonMessage(abandonCtx, message, nil)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to abandon message %s during processor close: %w", message.MessageID, err))
			continue
		}
		m.forget(message)
	}
	return errors.Join(errs...)
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
