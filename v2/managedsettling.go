package shuttle

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// ManagedSettlingFunc is the signature of the message handler to implement when using the ManagedSettling middleware
type ManagedSettlingFunc func(ctx context.Context, message *azservicebus.ReceivedMessage) error

var _ Handler = (*ManagedSettler)(nil)

// ManagedSettler is a middleware that allows to reduce the message handler signature to ManagedSettlingFunc
type ManagedSettler struct {
	next    ManagedSettlingFunc
	options *ManagedSettlingOptions
}

func (m *ManagedSettler) Handle(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
	if err := m.next(ctx, message); err != nil {
		log(ctx, "error returned from the handler. Calling ManagedSettler error handler")
		m.handleError(ctx, settler, message, err)
		return
	}
	if err := settler.CompleteMessage(ctx, message, nil); err != nil {
		log(ctx, err)
		m.options.OnAbandoned(ctx, message, err)
		return
		// if we fail to complete the message, we log the error and let the message lock expire.
		// we cannot do more at this point.
	}
	m.options.OnCompleted(ctx, message)
}

// RetryDecision allows to provide custom retry decision.
type RetryDecision interface {
	// CanRetry inspects the error returned from the message handler, and the message itself to decide if it should be retried or not.
	CanRetry(err error, message *azservicebus.ReceivedMessage) bool
}

// MaxAttemptsRetryDecision defines how many delivery the handler allows before explicitly moving the message to the deadletter queue.
// This requires the MaxDeliveryCount from the queue or subscription to be higher than the MaxAttempts property.
// If the queue or subscription's MaxDeliveryCount is lower than MaxAttempts,
// service bus will move the message to the DLQ before the handler reaches the MaxAttempts.
type MaxAttemptsRetryDecision struct {
	MaxAttempts uint32
}

func (d *MaxAttemptsRetryDecision) CanRetry(_ error, message *azservicebus.ReceivedMessage) bool {
	return message.DeliveryCount < d.MaxAttempts
}

// RetryDelayStrategy can be implemented to provide custom delay retry strategies.
type RetryDelayStrategy interface {
	GetDelay(deliveryCount uint32) time.Duration
}

// ConstantDelayStrategy delays the message retry by the given duration
type ConstantDelayStrategy struct {
	Delay time.Duration
}

func (s *ConstantDelayStrategy) GetDelay(_ uint32) time.Duration {
	return s.Delay
}

// ManagedSettlingOptions allows to configure the ManagedSettling middleware
type ManagedSettlingOptions struct {
	// RetryDecision is invoked to decide whether an error should be retried.
	// the default is to retry 5 times before moving the message to the deadletter.
	RetryDecision RetryDecision
	// RetryDelayStrategy is invoked when a message handling does not complete successfully
	// and the RetryDecision decides to retry the message.
	// The handler will sleep for the time calculated by the delayStrategy before Abandoning the message.
	RetryDelayStrategy RetryDelayStrategy
	// OnAbandoned is invoked when the handler returns an error. It is invoked after the message is abandoned or deadlettered.
	OnAbandoned func(context.Context, *azservicebus.ReceivedMessage, error)
	// OnDeadLettered is invoked after the ManagedSettling dead-letters a message.
	// this occurs when the RetryDecision.CanRetry implementation returns false following an error returned by the handler
	// It is invoked after the message is abandoned or deadlettered.
	OnDeadLettered func(context.Context, *azservicebus.ReceivedMessage, error)
	// OnCompleted is a func that is invoked when the handler does not return any error. it is invoked after the message is completed.
	OnCompleted func(context.Context, *azservicebus.ReceivedMessage)
}

// NewManagedSettlingHandler allows to configure Retry decision logic and delay strategy.
// It also adapts the handler to let the user return an error from the handler, instead of a settlement.
// the settlement is inferred from the handler's return value.
// error -> abandon
// nil -> complete
// the RetryDecision can be overridden and can inspect the error returned to decide to retry the message or not.
// this allows to define error types that shouldn't be retried (and moved directly to the deadletter queue)
func NewManagedSettlingHandler(opts *ManagedSettlingOptions, handler ManagedSettlingFunc) *ManagedSettler {
	const (
		defaultRetryDecisionMaxAttempts = 5
		defaultDelay                    = 5 * time.Second
	)
	options := &ManagedSettlingOptions{
		RetryDecision:      &MaxAttemptsRetryDecision{MaxAttempts: defaultRetryDecisionMaxAttempts},
		RetryDelayStrategy: &ConstantDelayStrategy{Delay: defaultDelay},
		OnCompleted: func(ctx context.Context, message *azservicebus.ReceivedMessage) {
			if opts.OnCompleted != nil {
				opts.OnCompleted(ctx, message)
			}
		},
		OnAbandoned: func(ctx context.Context, message *azservicebus.ReceivedMessage, err error) {
			if opts.OnAbandoned != nil {
				opts.OnAbandoned(ctx, message, err)
			}
		},
		OnDeadLettered: func(ctx context.Context, message *azservicebus.ReceivedMessage, err error) {
			if opts.OnDeadLettered != nil {
				opts.OnDeadLettered(ctx, message, err)
			}
		},
	}
	if opts != nil {
		if opts.RetryDecision != nil {
			options.RetryDecision = opts.RetryDecision
		}
		if opts.RetryDelayStrategy != nil {
			options.RetryDelayStrategy = opts.RetryDelayStrategy
		}
	}
	return &ManagedSettler{
		next:    handler,
		options: options,
	}
}

func (m *ManagedSettler) handleError(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage, handleErr error) {
	if handleErr == nil {
		handleErr = fmt.Errorf("nil error: %w", handleErr)
	}
	if !m.options.RetryDecision.CanRetry(handleErr, message) {
		log(ctx, "moving message to dead letter queue because processing failed to an error: %s", handleErr)
		deadLetterSettlement.settle(ctx, settler, message, &azservicebus.DeadLetterOptions{
			Reason:             to.Ptr("ManagedSettlingHandlerDeadLettering"),
			ErrorDescription:   to.Ptr(handleErr.Error()),
			PropertiesToModify: nil,
		})
		// this could be a special hook to have more control on deadlettering, but keeping it simple for now
		m.options.OnDeadLettered(ctx, message, handleErr)
		return
	}
	// the delay is implemented as an in-memory sleep before calling abandon.
	// this will continue renewing the lock on the message while we wait for this delay to pass.
	delay := m.options.RetryDelayStrategy.GetDelay(message.DeliveryCount)
	log(ctx, "delay strategy return delay of %s", delay)
	time.Sleep(delay)
	abandonSettlement.settle(ctx, settler, message, nil)
	m.options.OnAbandoned(ctx, message, handleErr)
}
