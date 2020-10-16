package message

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
)

// HandleFunc is a func to handle the message received from a subscription
type HandleFunc func(ctx context.Context, message *Message) Handler

func (h HandleFunc) Do(ctx context.Context, _ Handler, msg *servicebus.Message) Handler {
	wrapped, err := New(msg)
	if err != nil {
		return Error(err)
	}
	return h(ctx, wrapped)
}

type Handler interface {
	Do(ctx context.Context, orig Handler, message *servicebus.Message) Handler
}
