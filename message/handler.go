package message

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/devigned/tab"
)

// HandleFunc is a func to handle the message received from a subscription
type HandleFunc func(ctx context.Context, message *Message) Handler

// Do wraps a service message and returns a handlers
func (h HandleFunc) Do(ctx context.Context, _ Handler, msg *servicebus.Message) Handler {
	ctx, span := startSpanFromMessageAndContext(ctx, "go-shuttle.HandlerFunc.Do", msg)
	defer span.End()

	wrapped, err := New(msg)
	if err != nil {
		tab.For(ctx).Debug(err.Error())
		return Error(err)
	}
	return h(ctx, wrapped)
}

// Handler is the interface to handle messages
type Handler interface {
	Do(ctx context.Context, orig Handler, message *servicebus.Message) Handler
}
