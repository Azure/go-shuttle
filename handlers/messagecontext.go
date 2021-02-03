package handlers

import (
	"context"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
)

var _ servicebus.Handler = (*DeadlineContext)(nil)

// DeadlineContext is a servicebus middleware handler that creates a message context from the incoming context to isolate
// each message handling sub processes. It adds a deadline equal to `EnqueuedTime + TTL` to the message context if the message TTL is defined.
// Otherwise no deadline is applied on the message context.
// The context is canceled once the `next` middleware returns. this implies that no middleware above DeadlineContext can use this context.
type DeadlineContext struct {
	next servicebus.Handler
}

func (mc *DeadlineContext) Handle(ctx context.Context, msg *servicebus.Message) error {
	if mc.next == nil {
		return NextHandlerNilError
	}
	msgCtx, cancel := setupDeadline(ctx, msg)
	// NOTE: this assumes that no handler above the DeadlineContext handler needs this context.
	defer cancel()
	return mc.next.Handle(msgCtx, msg)
}

func NewDeadlineContext(next servicebus.Handler) *DeadlineContext {
	return &DeadlineContext{
		next: next,
	}
}

func setupDeadline(ctx context.Context, msg *servicebus.Message) (context.Context, func()) {
	msgCtx, cancel := context.WithCancel(ctx)
	if hasDeadline(msg) {
		deadline := msg.SystemProperties.EnqueuedTime.Add(*msg.TTL)
		msgCtx, _ = context.WithDeadline(ctx, deadline)
	}
	return msgCtx, cancel
}

func hasDeadline(msg *servicebus.Message) bool {
	return msg.SystemProperties != nil &&
		msg.SystemProperties.EnqueuedTime != nil &&
		msg.TTL != nil &&
		*msg.TTL > 0*time.Second
}
