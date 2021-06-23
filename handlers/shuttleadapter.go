package handlers

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/message"
	"github.com/devigned/tab"
)

var _ servicebus.Handler = (*shuttleAdapter)(nil)

// ShuttleAdapter wraps a go-shuttle message.Handler in a servicebus.Handler.
// It allows the consumer to write go-shuttle handler func
// that uses the go-shuttle semantic and message disposition API
type shuttleAdapter struct {
	next message.Handler
}

func NewShuttleAdapter(next message.Handler) servicebus.Handler {
	if next == nil {
		panic(NextHandlerNilError.Error())
	}
	return &shuttleAdapter{next: next}
}

func (c *shuttleAdapter) Handle(ctx context.Context, msg *servicebus.Message) error {
	ctx, s := tab.StartSpan(ctx, "go-shuttle.listener.shuttleAdapter.Handle")
	defer s.End()
	currentHandler := c.next
	for !message.IsDone(currentHandler) {
		currentHandler = currentHandler.Do(ctx, c.next, msg)
		// handle nil as a Completion!
		if currentHandler == nil {
			currentHandler = message.Complete()
		}
	}
	return nil
}
