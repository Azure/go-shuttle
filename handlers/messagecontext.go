package handlers

import (
	"context"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
)

var _ servicebus.Handler = (*deadlineContext)(nil)

// DeadlineContext is a servicebus middleware handler that creates a message context from the incoming context to isolate
// each message handling sub processes. It adds a deadline equal to `EnqueuedTime + TTL` to the message context if the message TTL is defined.
// Otherwise no deadline is applied on the message context.
// The context is canceled once the `next` middleware returns. this implies that no middleware above DeadlineContext can use this context.
type deadlineContext struct {
	next servicebus.Handler
}

func (mc *deadlineContext) Handle(ctx context.Context, msg *servicebus.Message) error {
	msgCtx, cancel := setupDeadline(ctx, msg)
	// NOTE: this assumes that no handler above the DeadlineContext handler needs this context.
	defer cancel()
	return mc.next.Handle(msgCtx, msg)
}

func NewDeadlineContext(next servicebus.Handler) servicebus.Handler {
	if next == nil {
		panic(NextHandlerNilError.Error())
	}
	return &deadlineContext{
		next: next,
	}
}

func setupDeadline(ctx context.Context, msg *servicebus.Message) (context.Context, func()) {
	if hasDeadline(msg) {
		deadline := msg.SystemProperties.EnqueuedTime.Add(*msg.TTL)
		return context.WithDeadline(ctx, deadline)
	}
	return context.WithCancel(ctx)
}

func hasDeadline(msg *servicebus.Message) bool {
	return msg.SystemProperties != nil &&
		msg.SystemProperties.EnqueuedTime != nil &&
		msg.TTL != nil &&
		*msg.TTL > 0*time.Second
}
