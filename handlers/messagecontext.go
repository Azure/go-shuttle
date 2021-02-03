package handlers

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
)

var _ servicebus.Handler = (*MessageContext)(nil)

// MessageContext is a servicebus middleware handler that creates a message context from the incoming context to isolate
// each message handling sub processes. It adds a deadline equal to `EnqueuedTime + TTL` to the message context if the message TTL is defined.
// Otherwise no deadline is applied on the message context.
type MessageContext struct {
	next servicebus.Handler
}

func (mc *MessageContext) Handle(ctx context.Context, msg *servicebus.Message) error {
	msgCtx := messageContext(ctx, msg)
	if mc.next != nil {
		return mc.next.Handle(msgCtx, msg)
	}
	return nil
}

func NewMessageContext(next servicebus.Handler) *MessageContext {
	return &MessageContext{
		next: next,
	}
}

func messageContext(ctx context.Context, msg *servicebus.Message) context.Context {
	msgCtx, _ := context.WithCancel(ctx)
	if hasDeadline(msg) {
		deadline := msg.SystemProperties.EnqueuedTime.Add(*msg.TTL)
		msgCtx, _ = context.WithDeadline(ctx, deadline)
	}
	return msgCtx
}
