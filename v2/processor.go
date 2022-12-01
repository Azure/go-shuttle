package v2

import (
	"context"
	"errors"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// MessageSettler is passed to the handlers. it exposes the message settling functionality from the receiver needed within the handler.
type MessageSettler interface {
	AbandonMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.AbandonMessageOptions) error
	CompleteMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.CompleteMessageOptions) error
	DeadLetterMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeadLetterOptions) error
	DeferMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeferMessageOptions) error
	RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error
}

// HandlerFunc is a func to handle the message received from a subscription
type HandlerFunc func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage)

// LockRenewer abstracts the servicebus receiver client to only expose lock renewal
type LockRenewer interface {
	RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error
}

// Processor encapsulates the message pump and concurrency handling of servicebus.
// it exposes a handler API to provides a middleware based message processing pipeline.
type Processor struct {
	receiver          *azservicebus.Receiver
	options           ProcessorOptions
	handle            HandlerFunc
	concurrencyTokens chan struct{} // tracks how many concurrent messages are currently being handled by the processor
}

// ProcessorOptions configures the processor
type ProcessorOptions struct {
	MaxConcurrency int
}

func NewProcessor(receiver *azservicebus.Receiver, handler HandlerFunc, options *ProcessorOptions) *Processor {
	return &Processor{
		receiver: receiver,
		handle:   handler,
		options:  *options,
	}
}

// Start starts the processor and blocks until an error occurs or the context is canceled.
func (p *Processor) Start(ctx context.Context) error {
	for ctx.Err() == nil {
		maxMessages := p.options.MaxConcurrency - len(p.concurrencyTokens)
		msgs, err := p.receiver.ReceiveMessages(ctx, maxMessages, &azservicebus.ReceiveMessagesOptions{})
		if err != nil {
			return err
		}
		for _, msg := range msgs {
			p.process(ctx, msg)
		}
	}
	return ctx.Err()
}

func (p *Processor) process(ctx context.Context, message *azservicebus.ReceivedMessage) {
	defer func() {
		<-p.concurrencyTokens
	}()
	p.concurrencyTokens <- struct{}{}
	go func() {
		msgContext, cancel := context.WithCancel(ctx)
		defer cancel()
		p.handle(msgContext, p.receiver, message)
	}()
}

// NewPanicHandler recovers panics from downstream handlers
func NewPanicHandler(handler HandlerFunc) HandlerFunc {
	defer func() {
		recover()
	}()
	return func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
		handler(ctx, settler, message)
	}
}

// NewRenewLockHandler starts a renewlock goroutine for each message received.
func NewRenewLockHandler(lockRenewer LockRenewer, interval *time.Duration, handler HandlerFunc) HandlerFunc {
	plr := &peekLockRenewer{
		next:            handler,
		lockRenewer:     lockRenewer,
		renewalInterval: interval,
	}
	return func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
		go plr.startPeriodicRenewal(ctx, message)
		handler(ctx, settler, message)
	}
}

// PeekLockRenewer starts a background goroutine that renews the message lock at the given interval until Stop() is called
// or until the passed in context is canceled.
// it is a pass through handler if the renewalInterval is nil
type peekLockRenewer struct {
	next            HandlerFunc
	lockRenewer     LockRenewer
	renewalInterval *time.Duration
}

func (plr *peekLockRenewer) startPeriodicRenewal(ctx context.Context, message *azservicebus.ReceivedMessage) {
	// _, span := tracing.StartSpanFromMessageAndContext(ctx, "go-shuttle.peeklock.startPeriodicRenewal", message)
	// defer span.End()
	count := 0
	for alive := true; alive; {
		select {
		case <-time.After(*plr.renewalInterval):
			count++
			// tab.For(ctx).Debug("Renewing message lock", tab.Int64Attribute("count", int64(count)))
			err := plr.lockRenewer.RenewMessageLock(ctx, message, nil)
			if err != nil {
				// listener.Metrics.IncMessageLockRenewedFailure(message)
				// I don't think this is a problem. the context is canceled when the message processing is over.
				// this can happen if we already entered the interval case when the message is completing.
				// tab.For(ctx).Info("failed to renew the peek lock", tab.StringAttribute("reason", err.Error()))
				return
			}
			// tab.For(ctx).Debug("renewed lock success")
			// listener.Metrics.IncMessageLockRenewedSuccess(message)
		case <-ctx.Done():
			// tab.For(ctx).Info("Stopping periodic renewal")
			err := ctx.Err()
			if errors.Is(err, context.DeadlineExceeded) {
				// listener.Metrics.IncMessageDeadlineReachedCount(message)
			}
			alive = false
		}
	}
}
