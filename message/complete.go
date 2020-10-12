package message

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
)

// Complete will notify Azure Service Bus that the message was successfully handled and should be deleted from the queue
func Complete() Handler {
	return &complete{}
}

type complete struct {
}

func (a *complete) Do(ctx context.Context, _ Handler, message *servicebus.Message) Handler {
	if err := message.Complete(ctx); err != nil {
		return Error(err)
	}
	return done()
}
