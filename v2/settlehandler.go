package shuttle

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/devigned/tab"
)

// Settlement represents an action to take on a message. Abandon, Complete, DeadLetter, Defer, NoOp
type Settlement interface {
	Settle(context.Context, MessageSettler, *azservicebus.ReceivedMessage)
}

// Abandon settlement will cause a message to be available again from the queue or subscription.
// This will increment its delivery count, and potentially cause it to be dead-lettered
// depending on your queue or subscription's configuration.
type Abandon struct {
	options *azservicebus.AbandonMessageOptions
}

func (a *Abandon) Settle(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
	abandonSettlement.settle(ctx, settler, message, a.options)
}

// Complete settlement completes a message, deleting it from the queue or subscription.
type Complete struct {
	options *azservicebus.CompleteMessageOptions
}

func (a *Complete) Settle(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
	completeSettlement.settle(ctx, settler, message, a.options)
}

// DeadLetter settlement moves the message to the dead letter queue for a
// queue or subscription. To process deadlettered messages, create a receiver with `Client.NewReceiverForQueue()`
// or `Client.NewReceiverForSubscription()` using the `ReceiverOptions.SubQueue` option.
type DeadLetter struct {
	options *azservicebus.DeadLetterOptions
}

func (a *DeadLetter) Settle(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
	deadLetterSettlement.settle(ctx, settler, message, a.options)
}

// Defer settlement will cause a message to be deferred. Deferred messages are moved to ta deferred queue. They can only be received using
// `Receiver.ReceiveDeferredMessages`.
type Defer struct {
	options *azservicebus.DeferMessageOptions
}

func (a *Defer) Settle(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
	deferSettlement.settle(ctx, settler, message, a.options)
}

// NoOp settlement exits the handler without taking an action, letting the message's peek lock expire before incrementing the
// delivery count, or moving it to the deadletter, depending on the queue or subscription's configuration
type NoOp struct {
}

func (a *NoOp) Settle(ctx context.Context, _ MessageSettler, message *azservicebus.ReceivedMessage) {
	span := tab.FromContext(ctx)
	getLogger(ctx).Warn(fmt.Sprintf("no op settlement. message lock is locked until: %s", message.LockedUntil))
	span.Logger().Info("no op settlement. message lock is not released")
}

type Settler func(ctx context.Context, message *azservicebus.ReceivedMessage) Settlement

func (s Settler) Handle(ctx context.Context, message *azservicebus.ReceivedMessage) Settlement {
	return s(ctx, message)
}

// SettlementHandlerOptions allows to configure the SettleHandler
type SettlementHandlerOptions struct {
	// OnNilSettlement is a func that allows to handle cases where the downstream handler returns nil.
	// the default behavior is to panic.
	OnNilSettlement func() Settlement
}

// NewSettlementHandler creates a middleware to use the Settlement api in the message handler implementation.
func NewSettlementHandler(opts *SettlementHandlerOptions, handler Settler) HandlerFunc {
	options := &SettlementHandlerOptions{
		OnNilSettlement: func() Settlement {
			panic("you must return a settlement from the message handler " +
				"or override the OnNilSettlement option to handle nil Settlement")
		},
	}
	if opts != nil {
		options.OnNilSettlement = opts.OnNilSettlement
	}
	return func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
		s := handler(ctx, message)
		if s == nil {
			// this panics unless the user overrides the OnNilSettlement option
			s = options.OnNilSettlement()
		}
		s.Settle(ctx, settler, message)
	}
}

type settlement[T any] struct {
	settleFunc func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage, options T) error
	name       string
}

func (s settlement[T]) settle(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage, options T) {
	span := tab.FromContext(ctx)
	span.Logger().Info(fmt.Sprintf("%s message", s.name))
	if err := s.settleFunc(ctx, settler, message, options); err != nil {
		wrapped := fmt.Errorf("%s settlement failed: %w", s.name, err)
		getLogger(ctx).Error(wrapped.Error())
		span.Logger().Error(wrapped)
		// the processing will terminate and the lock on the message will eventually be released after
		// the message lock expires on the broker side
	}
}

var abandonSettlement = settlement[*azservicebus.AbandonMessageOptions]{
	name: "abandon",
	settleFunc: func(ctx context.Context,
		settler MessageSettler,
		message *azservicebus.ReceivedMessage,
		options *azservicebus.AbandonMessageOptions) error {
		return settler.AbandonMessage(ctx, message, options)
	},
}

var completeSettlement = settlement[*azservicebus.CompleteMessageOptions]{
	name: "complete",
	settleFunc: func(ctx context.Context,
		settler MessageSettler,
		message *azservicebus.ReceivedMessage,
		options *azservicebus.CompleteMessageOptions) error {
		return settler.CompleteMessage(ctx, message, options)
	},
}

var deferSettlement = settlement[*azservicebus.DeferMessageOptions]{
	name: "defer",
	settleFunc: func(ctx context.Context,
		settler MessageSettler,
		message *azservicebus.ReceivedMessage,
		options *azservicebus.DeferMessageOptions) error {
		return settler.DeferMessage(ctx, message, options)
	},
}

var deadLetterSettlement = settlement[*azservicebus.DeadLetterOptions]{
	name: "dead letter",
	settleFunc: func(ctx context.Context,
		settler MessageSettler,
		message *azservicebus.ReceivedMessage,
		options *azservicebus.DeadLetterOptions) error {
		return settler.DeadLetterMessage(ctx, message, options)
	},
}
