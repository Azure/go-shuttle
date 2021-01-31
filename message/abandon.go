package message

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/tracing"
)

// Abandon stops processing the message and releases the lock on it.
func Abandon() Handler {
	return &abandon{}
}

type abandon struct {
}

func (a *abandon) Do(ctx context.Context, _ Handler, message *servicebus.Message) Handler {
	ctx, span := tracing.StartSpanFromMessageAndContext(ctx, "go-shuttle.abandon.Do", message)
	defer span.End()

	if err := message.Abandon(ctx); err != nil {
		span.Logger().Error(err)
		// the processing will terminate and the lock on the message will eventually be released after
		// the message lock expires on the broker side
	}
	return done()
}
