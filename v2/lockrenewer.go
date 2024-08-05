package shuttle

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Azure/go-shuttle/v2/metrics/processor"
)

// LockRenewer abstracts the servicebus receiver client to only expose lock renewal
type LockRenewer interface {
	RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error
}

// LockRenewalOptions configures the lock renewal.
type LockRenewalOptions struct {
	// Interval defines the frequency at which we renew the lock on the message. Defaults to 10 seconds.
	Interval *time.Duration
	// LockRenewalTimeout is the timeout value used on the context when sending RenewMessageLock() request.
	// Defaults to 5 seconds if not set or 0. Defaults to Lock Expiry time if set to a negative value.
	LockRenewalTimeout *time.Duration
	// CancelMessageContextOnStop will cancel the downstream message context when the renewal handler is stopped.
	// Defaults to true.
	CancelMessageContextOnStop *bool
	// MetricRecorder allows to pass a custom metric recorder for the LockRenewer.
	// Defaults to processor.Metric instance.
	MetricRecorder processor.Recorder
}

// NewRenewLockHandler returns a middleware handler that will renew the lock on the message at the specified interval.
func NewRenewLockHandler(options *LockRenewalOptions, handler Handler) HandlerFunc {
	interval := 10 * time.Second
	lockRenewalTimeout := 5 * time.Second
	cancelMessageContextOnStop := true
	metricRecorder := processor.Metric
	if options != nil {
		if options.Interval != nil {
			interval = *options.Interval
		}
		if options.LockRenewalTimeout != nil && *options.LockRenewalTimeout != 0 {
			lockRenewalTimeout = *options.LockRenewalTimeout
		}
		if options.CancelMessageContextOnStop != nil {
			cancelMessageContextOnStop = *options.CancelMessageContextOnStop
		}
		if options.MetricRecorder != nil {
			metricRecorder = options.MetricRecorder
		}
	}
	return func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
		plr := &peekLockRenewer{
			next:                   handler,
			lockRenewer:            settler,
			renewalInterval:        &interval,
			renewalTimeout:         &lockRenewalTimeout,
			metrics:                metricRecorder,
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

// Deprecated: use NewRenewLockHandler
// NewLockRenewalHandler returns a middleware handler that will renew the lock on the message at the specified interval.
func NewLockRenewalHandler(lockRenewer LockRenewer, options *LockRenewalOptions, handler Handler) HandlerFunc {
	return NewRenewLockHandler(options, handler)
}

// peekLockRenewer starts a background goroutine that renews the message lock at the given interval until Stop() is called
// or until the passed in context is canceled.
// it is a pass through handler if the renewalInterval is nil
type peekLockRenewer struct {
	next                   Handler
	lockRenewer            LockRenewer
	renewalInterval        *time.Duration
	renewalTimeout         *time.Duration
	metrics                processor.Recorder
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
	logger := getLogger(ctx)
	plr.alive.Store(false)
	// don't send the stop signal to the loop if there is already one in the channel
	if len(plr.stopped) == 0 {
		plr.stopped <- struct{}{}
	}
	if plr.cancelMessageCtxOnStop {
		logger.Info("canceling message context")
		plr.cancelMessageCtx()
	}
	logger.Info("stopped periodic renewal")
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
	logger := getLogger(ctx)
	count := 0
	span := trace.SpanFromContext(ctx)
	for plr.alive.Store(true); plr.alive.Load(); {
		select {
		case <-time.After(*plr.renewalInterval):
			if !plr.alive.Load() {
				return
			}
			count++
			err := plr.renewMessageLock(ctx, message, nil)
			if err != nil {
				// The context is canceled when the message handler returns from the processor.
				// This can happen if we already entered the interval case when the message processing completes.
				// The best we can do is log and retry on the next tick. The sdk already retries operations on recoverable network errors.
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					// if the error is a context error
					// we stop and let the next loop iteration handle the exit.
					plr.stop(ctx)
					continue
				}
				plr.metrics.IncMessageLockRenewedFailure(message)
				// on error, we continue to the next loop iteration.
				// if the context is Done, we will enter the ctx.Done() case and exit the renewal.
				// if the error is identified as permanent, we stop the renewal.
				// if the error is anything else, we keep trying the renewal.
				if plr.isPermanent(err) {
					logger.Error(fmt.Sprintf("stopping periodic renewal for message: %s", message.MessageID))
					plr.stop(ctx)
				}
				continue
			}
			span.AddEvent("message lock renewed", trace.WithAttributes(attribute.Int("count", count)))
			plr.metrics.IncMessageLockRenewedSuccess(message)
		case <-ctx.Done():
			logger.Info("context done: stopping periodic renewal")
			span.AddEvent("context done: stopping message lock renewal")
			err := ctx.Err()
			if errors.Is(err, context.DeadlineExceeded) {
				span.RecordError(err)
				plr.metrics.IncMessageDeadlineReachedCount(message)
			}
			plr.stop(ctx)
		case <-plr.stopped:
			if plr.alive.Load() {
				logger.Info("stop signal received: exiting periodic renewal")
				plr.alive.Store(false)
			}
		}
	}
}

func (plr *peekLockRenewer) renewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error {
	span := trace.SpanFromContext(ctx)
	lockLostErr := &azservicebus.Error{Code: azservicebus.CodeLockLost}
	if message.LockedUntil == nil || time.Until(*message.LockedUntil) < 0 {
		// if the lock doesn't exist or is already expired, we should not attempt to renew it.
		return lockLostErr
	}
	renewalTimeout := time.Until(*message.LockedUntil)
	if *plr.renewalTimeout > 0 {
		renewalTimeout = *plr.renewalTimeout
	}
	// we should keep retrying until lock expiry or until message context is done
	for time.Until(*message.LockedUntil) > 0 && ctx.Err() == nil {
		var renewErr error
		func() {
			getLogger(ctx).Info(fmt.Sprintf("renewing lock with timeout: %s", renewalTimeout))
			renewalCtx, cancel := context.WithTimeout(ctx, renewalTimeout)
			defer cancel()
			renewErr = plr.lockRenewer.RenewMessageLock(renewalCtx, message, options)
		}()
		if renewErr != nil {
			getLogger(ctx).Error(fmt.Sprintf("failed to renew lock: %s", renewErr))
			span.RecordError(fmt.Errorf("failed to renew lock: %w", renewErr))
		}
		// exit the renewal if no error or if we get any error other than context deadline exceeded
		if !errors.Is(renewErr, context.DeadlineExceeded) {
			return renewErr
		}
		getLogger(ctx).Error(fmt.Sprintf("renewal error is %s, retrying the renewal fast", renewErr))
	}
	// lock is expired or message context is done
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return lockLostErr
}
