package shuttle

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	receivers         []Receiver
	options           ProcessorOptions
	handle            Handler
	concurrencyTokens chan struct{} // tracks how many concurrent messages are currently being handled by the processor, shared across all receivers
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
	StartRetryDelayStrategy RetryDelayStrategy
}

func NewProcessor(receiver Receiver, handler HandlerFunc, options *ProcessorOptions) *Processor {
	opts := ProcessorOptions{
		MaxConcurrency:          1,
		ReceiveInterval:         to.Ptr(1 * time.Second),
		StartMaxAttempt:         1,
		StartRetryDelayStrategy: &ConstantDelayStrategy{Delay: 5 * time.Second},
	}
	if options != nil {
		if options.ReceiveInterval != nil {
			opts.ReceiveInterval = options.ReceiveInterval
		}
		if options.MaxConcurrency > 0 {
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
		receivers:         []Receiver{receiver},
		handle:            handler,
		options:           opts,
		concurrencyTokens: make(chan struct{}, opts.MaxConcurrency),
	}
}

func (p *Processor) AddReceiver(receivers ...Receiver) {
	p.receivers = append(p.receivers, receivers...)
}

// Start starts processing on all the receivers of the processor and blocks until all processors are stopped or the context is canceled.
// It will retry starting the processor based on the StartMaxAttempt and StartRetryDelayStrategy.
// Returns a combined list of errors encountered during each processor start.
func (p *Processor) Start(ctx context.Context) error {
	wg := sync.WaitGroup{}
	errChan := make(chan error, len(p.receivers))
	for i := range p.receivers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			err := p.StartAtIndex(ctx, index)
			if err != nil {
				errChan <- err
			}
		}(i)
	}
	wg.Wait()
	close(errChan)
	var allErrs []error
	for err := range errChan {
		allErrs = append(allErrs, err)
	}
	return errors.Join(allErrs...)
}

// StartAtIndex starts the processor with receiver index and blocks until an error occurs or the context is canceled.
// It will retry starting the processor based on the StartMaxAttempt and StartRetryDelayStrategy.
// Returns a combined list of errors during the start attempts or ctx.Err() if the context
// is cancelled during the retries.
func (p *Processor) StartAtIndex(ctx context.Context, index int) error {
	if index < 0 || index >= len(p.receivers) {
		return fmt.Errorf("index out of range: %d", index)
	}
	var savedError error
	for attempt := 0; attempt < p.options.StartMaxAttempt; attempt++ {
		if err := p.start(ctx, index); err != nil {
			savedError = errors.Join(savedError, err)
			log(ctx, fmt.Sprintf("processor %d start attempt %d failed: %v", index, attempt, err))
			if attempt+1 == p.options.StartMaxAttempt { // last attempt, return early
				break
			}
			select {
			case <-time.After(p.options.StartRetryDelayStrategy.GetDelay(uint32(attempt))):
				continue
			case <-ctx.Done():
				log(ctx, "context done, stop retrying")
				return ctx.Err()
			}
		}
	}
	return savedError
}

// start starts the processor and blocks until an error occurs or the context is canceled.
func (p *Processor) start(ctx context.Context, index int) error {
	receiver := p.receivers[index]
	log(ctx, fmt.Sprintf("starting processor %d", index))
	messages, err := receiver.ReceiveMessages(ctx, p.options.MaxConcurrency, nil)
	if err != nil {
		return fmt.Errorf("processor %d failed to receive messages: %w", index, err)
	}
	log(ctx, fmt.Sprintf("processor %d received %d messages - initial", index, len(messages)))
	processor.Metric.IncMessageReceived(float64(len(messages)))
	for _, msg := range messages {
		p.process(ctx, receiver, msg)
	}
	for ctx.Err() == nil {
		select {
		case <-time.After(*p.options.ReceiveInterval):
			maxMessages := p.options.MaxConcurrency - len(p.concurrencyTokens)
			if ctx.Err() != nil || maxMessages == 0 {
				break
			}
			messages, err := receiver.ReceiveMessages(ctx, maxMessages, nil)
			if err != nil {
				return fmt.Errorf("processor %d failed to receive messages: %w", index, err)
			}
			log(ctx, fmt.Sprintf("processor %d received %d messages from processor loop", index, len(messages)))
			processor.Metric.IncMessageReceived(float64(len(messages)))
			for _, msg := range messages {
				p.process(ctx, receiver, msg)
			}
		case <-ctx.Done():
			log(ctx, fmt.Sprintf("context done, stop receiving from processor %d", index))
			break
		}
	}
	log(ctx, fmt.Sprintf("exiting processor %d", index))
	return fmt.Errorf("processor %d stopped: %w", index, ctx.Err())
}

func (p *Processor) process(ctx context.Context, receiver Receiver, message *azservicebus.ReceivedMessage) {
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
		p.handle.Handle(msgContext, receiver, message)
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
