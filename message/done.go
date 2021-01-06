package message

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
)

var doneHandlerInstance = &doneHandler{}

// IsDone returns true and terminates processing if the handler is doneHandlerInstance.
func IsDone(handler Handler) bool {
	return handler == doneHandlerInstance
}

type doneHandler struct{}

func (a *doneHandler) Do(_ context.Context, _ Handler, _ *servicebus.Message, _ *servicebus.Subscription) Handler {
	panic("done Handler is not meant to be called, it marks the end of the processing")
}

func done() Handler {
	return doneHandlerInstance
}
