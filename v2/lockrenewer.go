package shuttle

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Azure/go-shuttle/v2/metrics"
)

// LockRenewer abstracts the servicebus receiver client to only expose lock renewal
type LockRenewer interface {
	RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error
}

// LockRenewalOptions configures the lock renewal.
type LockRenewalOptions struct {
	// Interval defines the frequency at which we renew the lock on the message. Defaults to 10 seconds.
	Interval *time.Duration
	// CancelMessageContextOnStop will cancel the downstream message context when the renewal handler is stopped.
	// Defaults to true.
	CancelMessageContextOnStop *bool
}

// NewLockRenewalHandler returns a middleware handler that will renew the lock on the message at the specified interval.
func NewLockRenewalHandler(lockRenewer LockRenewer, options *LockRenewalOptions, handler Handler) HandlerFunc {
	interval := 10 * time.Second
	cancelMessageContextOnStop := true
	if options != nil {
		if options.Interval != nil {
			interval = *options.Interval
		}
		if options.CancelMessageContextOnStop != nil {
			cancelMessageContextOnStop = *options.CancelMessageContextOnStop
		}
	}
	return func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
		plr := &peekLockRenewer{
			next:                   handler,
			lockRenewer:            lockRenewer,
			renewalInterval:        &interval,
			cancelMessageCtxOnStop: cancelMessageContextOnStop,
			stopped:                make(chan struct{}, 1), // buffered channel to ensure we are not blocking
		}
		renewalCtx, cancel := context.WithCancel(ctx)
		plr.cancelMessageCtx = cancel
		go plr.startPeriodicRenewal(renewalCtx, message)
		handler.Handle(renewalCtx, settler, message)
		plr.stop(renewalCtx)
	}
}

// Deprecated: use NewLockRenewalHandler
// NewRenewLockHandler starts a renewlock goroutine for each message received.
func NewRenewLockHandler(lockRenewer LockRenewer, interval *time.Duration, handler Handler) HandlerFunc {
	return NewLockRenewalHandler(lockRenewer,
		&LockRenewalOptions{
			Interval: interval,
			// default to false on the old handler signature to keep the same behavior
			CancelMessageContextOnStop: to.Ptr(false)},
		handler)
}

// peekLockRenewer starts a background goroutine that renews the message lock at the given interval until Stop() is called
// or until the passed in context is canceled.
// it is a pass through handler if the renewalInterval is nil
type peekLockRenewer struct {
	next                   Handler
	lockRenewer            LockRenewer
	renewalInterval        *time.Duration
	alive                  atomic.Bool
	cancelMessageCtxOnStop bool
	cancelMessageCtx       func()

	// stopped channel allows to short circuit the renewal loop
	// when we are already waiting on the select.
	// the channel is needed in addition to the boolean
	// to cover the case where we might have finished handling the message and called stop on the renewer
	// before the renewal goroutine had a chance to start.
	stopped chan struct{}
}

// stop will stop the renewal loop. if LockRenewalOptions.CancelMessageContextOnStop is set to true, it cancels the message context.
func (plr *peekLockRenewer) stop(ctx context.Context) {
	plr.alive.Store(false)
	// don't send the stop signal to the loop if there is already one in the channel
	if len(plr.stopped) == 0 {
		plr.stopped <- struct{}{}
	}
	if plr.cancelMessageCtxOnStop {
		log(ctx, "canceling message context")
		plr.cancelMessageCtx()
	}
	log(ctx, "stopped periodic renewal")
}

func (plr *peekLockRenewer) isPermanent(err error) bool {
	var sbErr *azservicebus.Error
	if errors.As(err, &sbErr) {
		// once the lock is lost, the renewal cannot succeed.
		return sbErr.Code == azservicebus.CodeLockLost ||
			sbErr.Code == azservicebus.CodeUnauthorizedAccess
	}
	return false
}

func (plr *peekLockRenewer) startPeriodicRenewal(ctx context.Context, message *azservicebus.ReceivedMessage) {
	count := 0
	span := trace.SpanFromContext(ctx)
	for plr.alive.Store(true); plr.alive.Load(); {
		select {
		case <-time.After(*plr.renewalInterval):
			if !plr.alive.Load() {
				return
			}
			log(ctx, "renewing lock")
			count++
			err := plr.lockRenewer.RenewMessageLock(ctx, message, nil)
			if err != nil {
				log(ctx, fmt.Sprintf("failed to renew lock: %s", err))
				metrics.Processor.IncMessageLockRenewedFailure(message)
				// The context is canceled when the message handler returns from the processor.
				// This can happen if we already entered the interval case when the message processing completes.
				// The best we can do is log and retry on the next tick. The sdk already retries operations on recoverable network errors.
				span.RecordError(fmt.Errorf("failed to renew lock: %w", err))
				// on error, we continue to the next loop iteration.
				// if the context is Done, we will enter the ctx.Done() case and exit the renewal.
				// if the error is identified as permanent, we stop the renewal.
				// if the error is anything else, we keep trying the renewal.
				if plr.isPermanent(err) {
					log(ctx, fmt.Sprintf("stopping periodic renewal for message: %s", message.MessageID))
					plr.stop(ctx)
				}
				continue
			}
			span.AddEvent("message lock renewed", trace.WithAttributes(attribute.Int("count", count)))
			metrics.Processor.IncMessageLockRenewedSuccess(message)
		case <-ctx.Done():
			log(ctx, "context done: stopping periodic renewal")
			span.AddEvent("context done: stopping message lock renewal")
			err := ctx.Err()
			if errors.Is(err, context.DeadlineExceeded) {
				span.RecordError(err)
				metrics.Processor.IncMessageDeadlineReachedCount(message)
			}
			plr.stop(ctx)
		case <-plr.stopped:
			if plr.alive.Load() {
				log(ctx, "stop signal received: exiting periodic renewal")
				plr.alive.Store(false)
			}
		}
	}
}
