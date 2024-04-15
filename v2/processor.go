package shuttle

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2/metrics/processor"
)

type Receiver interface {
	ReceiveMessages(ctx context.Context, maxMessages int, options *azservicebus.ReceiveMessagesOptions) ([]*azservicebus.ReceivedMessage, error)
	MessageSettler
}

// MessageSettler is passed to the handlers. it exposes the message settling functionality from the receiver needed within the handler.
type MessageSettler interface {
	AbandonMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.AbandonMessageOptions) error
	CompleteMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.CompleteMessageOptions) error
	DeadLetterMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeadLetterOptions) error
	DeferMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeferMessageOptions) error
	RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error
}

type Handler interface {
	Handle(context.Context, MessageSettler, *azservicebus.ReceivedMessage)
}

// HandlerFunc is a func to handle the message received from a subscription
type HandlerFunc func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage)

func (f HandlerFunc) Handle(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
	f(ctx, settler, message)
}

// Processor encapsulates the message pump and concurrency handling of servicebus.
// it exposes a handler API to provides a middleware based message processing pipeline.
type Processor struct {
	receiver          Receiver
	options           ProcessorOptions
	handle            Handler
	concurrencyTokens chan struct{} // tracks how many concurrent messages are currently being handled by the processor
}

// ProcessorOptions configures the processor
// MaxConcurrency defaults to 1. Not setting MaxConcurrency, or setting it to 0 or a negative value will fallback to the default.
// ReceiveInterval defaults to 2 seconds if not set.
// StartMaxAttempt defaults to 1 if not set (no retries). Not setting StartMaxAttempt, or setting it to non-positive value will fallback to the default.
// StartRetryDelayStrategy defaults to a fixed 5-second delay if not set.
type ProcessorOptions struct {
	MaxConcurrency  int
	ReceiveInterval *time.Duration

	// StartMaxAttempt is the maximum number of attempts to start the processor.
	StartMaxAttempt int
	// StartRetryDelay is the delay between each start attempt.
	StartRetryDelayStrategy StartRetryDelayStrategy
}

// StartRetryDelayStrategy can be implemented to provide custom delay strategies on connection retry.
type StartRetryDelayStrategy interface {
	GetDelay(attempt int) time.Duration
}

// FixedStartDelayStrategy is a fixed delay strategy for connection retry.
type FixedStartDelayStrategy struct {
	Delay time.Duration
}

func (f *FixedStartDelayStrategy) GetDelay(_ int) time.Duration {
	return f.Delay
}

func NewProcessor(receiver Receiver, handler HandlerFunc, options *ProcessorOptions) *Processor {
	opts := ProcessorOptions{
		MaxConcurrency:          1,
		ReceiveInterval:         to.Ptr(1 * time.Second),
		StartMaxAttempt:         1,
		StartRetryDelayStrategy: &FixedStartDelayStrategy{Delay: 5 * time.Second},
	}
	if options != nil {
		if options.ReceiveInterval != nil {
			opts.ReceiveInterval = options.ReceiveInterval
		}
		if options.MaxConcurrency >= 0 {
			opts.MaxConcurrency = options.MaxConcurrency
		}
		if options.StartMaxAttempt > 0 {
			opts.StartMaxAttempt = options.StartMaxAttempt
		}
		if options.StartRetryDelayStrategy != nil {
			opts.StartRetryDelayStrategy = options.StartRetryDelayStrategy
		}
	}
	return &Processor{
		receiver:          receiver,
		handle:            handler,
		options:           opts,
		concurrencyTokens: make(chan struct{}, opts.MaxConcurrency),
	}
}

// Start starts the processor and blocks until an error occurs or the context is canceled.
// It will retry starting the processor based on the StartMaxAttempt and StartRetryDelayStrategy.
// Returns the last error encountered while starting the processor.
func (p *Processor) Start(ctx context.Context) error {
	var savedError error
	for attempt := 0; attempt < p.options.StartMaxAttempt; attempt++ {
		if err := p.start(ctx); err != nil {
			savedError = err
			log(ctx, fmt.Sprintf("processor start attempt %d failed: %v", attempt, err))
			if attempt+1 == p.options.StartMaxAttempt { // last attempt, return early
				break
			}
			select {
			case <-time.After(p.options.StartRetryDelayStrategy.GetDelay(attempt)):
				continue
			case <-ctx.Done():
				log(ctx, "context done, stop retrying")
				return savedError
			}
		}
	}
	return savedError
}

// start starts the processor and blocks until an error occurs or the context is canceled.
func (p *Processor) start(ctx context.Context) error {
	log(ctx, "starting processor")
	messages, err := p.receiver.ReceiveMessages(ctx, p.options.MaxConcurrency, nil)
	if err != nil {
		return err
	}
	log(ctx, fmt.Sprintf("received %d messages - initial", len(messages)))
	processor.Metric.IncMessageReceived(float64(len(messages)))
	for _, msg := range messages {
		p.process(ctx, msg)
	}
	for ctx.Err() == nil {
		select {
		case <-time.After(*p.options.ReceiveInterval):
			maxMessages := p.options.MaxConcurrency - len(p.concurrencyTokens)
			if ctx.Err() != nil || maxMessages == 0 {
				break
			}
			messages, err := p.receiver.ReceiveMessages(ctx, maxMessages, nil)
			if err != nil {
				return err
			}
			log(ctx, fmt.Sprintf("received %d messages from processor loop", len(messages)))
			processor.Metric.IncMessageReceived(float64(len(messages)))
			for _, msg := range messages {
				p.process(ctx, msg)
			}
		case <-ctx.Done():
			log(ctx, "context done, stop receiving")
			break
		}
	}
	log(ctx, "exiting processor")
	return ctx.Err()
}

func (p *Processor) process(ctx context.Context, message *azservicebus.ReceivedMessage) {
	p.concurrencyTokens <- struct{}{}
	go func() {
		msgContext, cancel := context.WithCancel(ctx)
		// cancel messageContext when we get out of this goroutine
		defer cancel()
		defer func() {
			<-p.concurrencyTokens
			processor.Metric.IncMessageHandled(message)
			processor.Metric.DecConcurrentMessageCount(message)
		}()
		processor.Metric.IncConcurrentMessageCount(message)
		p.handle.Handle(msgContext, p.receiver, message)
	}()
}

type PanicHandlerOptions struct {
	OnPanicRecovered func(
		ctx context.Context,
		settler MessageSettler,
		message *azservicebus.ReceivedMessage,
		recovered any)
}

// NewPanicHandler recovers panics from downstream handlers
func NewPanicHandler(panicOptions *PanicHandlerOptions, handler Handler) HandlerFunc {
	if panicOptions == nil {
		panicOptions = &PanicHandlerOptions{
			OnPanicRecovered: func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage, recovered any) {
				log(ctx, recovered)
			},
		}
	}
	return func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
		defer func() {
			if rec := recover(); rec != nil {
				panicOptions.OnPanicRecovered(ctx, settler, message, rec)
			}
		}()
		handler.Handle(ctx, settler, message)
	}
}
