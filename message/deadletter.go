package message

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/tracing"
)

// DeadLetter will notify Azure Service Bus that the message will be sent to the deadletter queue
func DeadLetter(err error) Handler {
	return &deadLetter{err: err}
}

type deadLetter struct {
	err error
}

func (d *deadLetter) Do(ctx context.Context, _ Handler, message *servicebus.Message) Handler {
	ctx, span := tracing.StartSpanFromMessageAndContext(ctx, "go-shuttle.DeadLetter.Do", message)
	defer span.End()

	if err := message.DeadLetter(ctx, d.err); err != nil {
		span.Logger().Error(err)
		return Error(err)
	}
	return done()
}
