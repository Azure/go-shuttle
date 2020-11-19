package message

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/devigned/tab"
)

// Complete will notify Azure Service Bus that the message was successfully handled and should be deleted from the queue
func Complete() Handler {
	return &complete{}
}

type complete struct {
}

func (a *complete) Do(ctx context.Context, _ Handler, message *servicebus.Message) Handler {
	ctx, span := startSpanFromMessageAndContext(ctx, "go-shuttle.complete.Do", message)
	defer span.End()

	if err := message.Complete(ctx); err != nil {
		tab.For(ctx).Debug(err.Error())
		return Error(err)
	}
	return done()
}
