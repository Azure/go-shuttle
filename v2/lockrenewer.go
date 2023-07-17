package shuttle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Azure/go-shuttle/v2/metrics"
)

// NewRenewLockHandler starts a renewlock goroutine for each message received.
func NewRenewLockHandler(lockRenewer LockRenewer, interval *time.Duration, handler Handler) HandlerFunc {
	plr := &peekLockRenewer{
		next:            handler,
		lockRenewer:     lockRenewer,
		renewalInterval: interval,
	}
	return func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
		go plr.startPeriodicRenewal(ctx, message)
		handler.Handle(ctx, settler, message)
	}
}

// PeekLockRenewer starts a background goroutine that renews the message lock at the given interval until Stop() is called
// or until the passed in context is canceled.
// it is a pass through handler if the renewalInterval is nil
type peekLockRenewer struct {
	next            Handler
	lockRenewer     LockRenewer
	renewalInterval *time.Duration
}

func (plr *peekLockRenewer) startPeriodicRenewal(ctx context.Context, message *azservicebus.ReceivedMessage) {
	count := 0
	span := trace.SpanFromContext(ctx)
	for alive := true; alive; {
		select {
		case <-time.After(*plr.renewalInterval):
			log(ctx, "renewing lock")
			count++

			err := plr.lockRenewer.RenewMessageLock(ctx, message, nil)
			if err != nil {
				log(ctx, "failed to renew lock: ", err)
				metrics.Processor.IncMessageLockRenewedFailure(message)
				// The context is canceled when the message handler returns from the processor.
				// This can happen if we already entered the interval case when the message processing completes.
				// The best we can do is log and retry on the next tick. The sdk already retries operations on recoverable network errors.
				span.RecordError(fmt.Errorf("failed to renew lock: %w", err))
				// on error, we continue to the next loop iteration.
				// if the context is Done, we will enter the ctx.Done() case and exit the renewal.
				// if the error is anything else, we keep retrying the renewal
				continue
			}
			span.AddEvent("message lock renewed", trace.WithAttributes(attribute.Int("count", count)))
			metrics.Processor.IncMessageLockRenewedSuccess(message)
		case <-ctx.Done():
			log(ctx, ctx, "context done: stopping periodic renewal")
			span.AddEvent("context done: stopping message lock renewal")
			err := ctx.Err()
			if errors.Is(err, context.DeadlineExceeded) {
				span.RecordError(err)
				metrics.Processor.IncMessageDeadlineReachedCount(message)
			}
			alive = false
		}
	}
}
