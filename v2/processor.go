package shuttle

import (
	"context"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"

	"github.com/Azure/go-shuttle/v2/metrics"
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

// LockRenewer abstracts the servicebus receiver client to only expose lock renewal
type LockRenewer interface {
	RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error
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
type ProcessorOptions struct {
	MaxConcurrency  int
	ReceiveInterval *time.Duration
}

func NewProcessor(receiver Receiver, handler HandlerFunc, options *ProcessorOptions) *Processor {
	opts := ProcessorOptions{
		MaxConcurrency:  1,
		ReceiveInterval: to.Ptr(1 * time.Second),
	}
	if options != nil {
		if options.ReceiveInterval != nil {
			opts.ReceiveInterval = options.ReceiveInterval
		}
		if options.MaxConcurrency >= 0 {
			opts.MaxConcurrency = options.MaxConcurrency
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
func (p *Processor) Start(ctx context.Context) error {
	log(ctx, "starting processor")
	messages, err := p.receiver.ReceiveMessages(ctx, p.options.MaxConcurrency, nil)
	log(ctx, "received ", len(messages), " messages - initial")
	metrics.Processor.IncMessageReceived(float64(len(messages)))
	if err != nil {
		return err
	}
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
			log(ctx, "received ", len(messages), " messages from processor loop")
			metrics.Processor.IncMessageReceived(float64(len(messages)))
			if err != nil {
				return err
			}
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
			metrics.Processor.IncMessageHandled(message)
			metrics.Processor.DecConcurrentMessageCount(message)
		}()
		metrics.Processor.IncConcurrentMessageCount(message)
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
