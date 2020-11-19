package message

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/devigned/tab" // same way as service-bus to log errors
)

// Abandon stops processing the message and releases the lock on it.
func Abandon() Handler {
	return &abandon{}
}

type abandon struct {
}

func (a *abandon) Do(ctx context.Context, _ Handler, message *servicebus.Message) Handler {
	ctx, span := startSpanFromMessageAndContext(ctx, "go-shuttle.abandon.Do", message)
	defer span.End()

	if err := message.Abandon(ctx); err != nil {
		tab.For(ctx).Error(err)

		// the processing will terminate and the lock on the message will be released after messageLockDuration
	}
	return done()
}
