package shuttle

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// ManagedSettlingFunc allows to convert a function with the signature
// func(context.Context, *azservicebus.ReceivedMessage) error
// to the ManagedSettlingHandler interface.
type ManagedSettlingFunc func(ctx context.Context, message *azservicebus.ReceivedMessage) error

func (f ManagedSettlingFunc) Handle(ctx context.Context, message *azservicebus.ReceivedMessage) error {
	return f(ctx, message)
}

// ManagedSettlingHandler is the message Handler interface for the ManagedSettler.
type ManagedSettlingHandler interface {
	Handle(context.Context, *azservicebus.ReceivedMessage) error
}

var _ Handler = (*ManagedSettler)(nil)

// ManagedSettler is a middleware that allows to reduce the message handler signature to ManagedSettlingFunc
type ManagedSettler struct {
	next    ManagedSettlingHandler
	options *ManagedSettlingOptions
}

func (m *ManagedSettler) Handle(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
	logger := getLogger(ctx)
	if err := m.next.Handle(ctx, message); err != nil {
		logger.Error(fmt.Sprintf("error returned from the handler. Calling ManagedSettler error handler. error: %s", err))
		m.options.OnError(ctx, m.options, settler, message, err)
		return
	}
	if err := settler.CompleteMessage(ctx, message, nil); err != nil {
		logger.Error(fmt.Sprintf("error completing message: %s", err))
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

// ManagedSettlingOptions allows to configure the ManagedSettling middleware
type ManagedSettlingOptions struct {
	// Allows to override the built-in error handling logic.
	// OnError is called before any message settling action is taken.
	// the ManagedSettlingOptions struct is passed as an argument so that the configuration
	// like RetryDecision, RetryDelayStrategy and the post-settlement hooks can be reused and composed differently
	OnError func(ctx context.Context, opts *ManagedSettlingOptions, settler MessageSettler, message *azservicebus.ReceivedMessage, handleErr error)
	// RetryDecision is invoked to decide whether an error should be retried.
	// the default is to retry 5 times before moving the message to the deadletter.
	RetryDecision RetryDecision
	// RetryDelayStrategy is invoked when a message handling does not complete successfully
	// and the RetryDecision decides to retry the message.
	// The handler will sleep for the time calculated by the delayStrategy before Abandoning the message.
	RetryDelayStrategy RetryDelayStrategy
	// OnAbandoned is invoked when the handler returns an error. It is invoked after the message is abandoned.
	OnAbandoned func(context.Context, *azservicebus.ReceivedMessage, error)
	// OnDeadLettered is invoked after the ManagedSettling dead-letters a message.
	// this occurs when the RetryDecision.CanRetry implementation returns false following an error returned by the handler
	// It is invoked after the message is dead-lettered.
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
func NewManagedSettlingHandler(opts *ManagedSettlingOptions, handler ManagedSettlingHandler) *ManagedSettler {
	options := defaultManagedSettlingOptions()
	if opts != nil {
		if opts.OnError != nil {
			options.OnError = opts.OnError
		}
		if opts.RetryDecision != nil {
			options.RetryDecision = opts.RetryDecision
		}
		if opts.RetryDelayStrategy != nil {
			options.RetryDelayStrategy = opts.RetryDelayStrategy
		}
		if opts.OnCompleted != nil {
			options.OnCompleted = opts.OnCompleted
		}
		if opts.OnAbandoned != nil {
			options.OnAbandoned = opts.OnAbandoned
		}
		if opts.OnDeadLettered != nil {
			options.OnDeadLettered = opts.OnDeadLettered
		}
	}
	return &ManagedSettler{
		next:    handler,
		options: options,
	}
}

func defaultManagedSettlingOptions() *ManagedSettlingOptions {
	const (
		defaultRetryDecisionMaxAttempts = 5
		defaultDelay                    = 5 * time.Second
	)
	return &ManagedSettlingOptions{
		OnError:            handleError,
		RetryDecision:      &MaxAttemptsRetryDecision{MaxAttempts: defaultRetryDecisionMaxAttempts},
		RetryDelayStrategy: &ConstantDelayStrategy{Delay: defaultDelay},
		OnCompleted: func(_ context.Context, _ *azservicebus.ReceivedMessage) {
		},
		OnAbandoned: func(_ context.Context, _ *azservicebus.ReceivedMessage, _ error) {
		},
		OnDeadLettered: func(_ context.Context, _ *azservicebus.ReceivedMessage, _ error) {
		},
	}
}

func handleError(ctx context.Context,
	options *ManagedSettlingOptions,
	settler MessageSettler,
	message *azservicebus.ReceivedMessage,
	handleErr error) {
	if handleErr == nil {
		handleErr = fmt.Errorf("nil error: %w", handleErr)
	}
	logger := getLogger(ctx)
	if !options.RetryDecision.CanRetry(handleErr, message) {
		logger.Error(fmt.Sprintf("moving message to dead letter queue because processing failed to an error: %s", handleErr))
		deadLetterSettlement.settle(ctx, settler, message, &azservicebus.DeadLetterOptions{
			Reason:             to.Ptr("ManagedSettlingHandlerDeadLettering"),
			ErrorDescription:   to.Ptr(handleErr.Error()),
			PropertiesToModify: nil,
		})
		// this could be a special hook to have more control on deadlettering, but keeping it simple for now
		options.OnDeadLettered(ctx, message, handleErr)
		return
	}
	// the delay is implemented as an in-memory sleep before calling abandon.
	// this will continue renewing the lock on the message while we wait for this delay to pass.
	delay := options.RetryDelayStrategy.GetDelay(message.DeliveryCount)
	logger.Error(fmt.Sprintf("delay strategy return delay of %s", delay))
	time.Sleep(delay)
	abandonSettlement.settle(ctx, settler, message, nil)
	options.OnAbandoned(ctx, message, handleErr)
}
