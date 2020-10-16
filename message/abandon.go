package message

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
)

// Abandon stops processing the message and releases the lock on it.
func Abandon() Handler {
	return &abandon{}
}

type abandon struct {
}

func (a *abandon) Do(ctx context.Context, _ Handler, message *servicebus.Message) Handler {
	if err := message.Abandon(ctx); err != nil {
		// trace the failure to abandon the message.
		// the processing will terminate and the lock on the message will be released after messageLockDuration
	}
	return done()
}
