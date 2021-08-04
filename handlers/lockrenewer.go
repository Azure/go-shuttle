package handlers

import (
	"context"
	"errors"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/prometheus/listener"
	"github.com/Azure/go-shuttle/tracing"
	"github.com/devigned/tab"
)

var _ servicebus.Handler = (*peekLockRenewer)(nil)

// LockRenewer abstracts the servicebus subscription client where this functionality lives
type LockRenewer interface {
	RenewLocks(ctx context.Context, messages ...*servicebus.Message) error
}

// PeekLockRenewer starts a background goroutine that renews the message lock at the given interval until Stop() is called
// or until the passed in context is canceled.
// it is a pass through handler if the renewalInterval is nil
type peekLockRenewer struct {
	next            servicebus.Handler
	lockRenewer     LockRenewer
	renewalInterval *time.Duration
}

func (plr *peekLockRenewer) Handle(ctx context.Context, msg *servicebus.Message) error {
	if plr.lockRenewer != nil && plr.renewalInterval != nil {
		go plr.startPeriodicRenewal(ctx, msg)
	}
	return plr.next.Handle(ctx, msg)
}

func NewPeekLockRenewer(interval *time.Duration, lockrenewer LockRenewer, next servicebus.Handler) servicebus.Handler {
	if next == nil {
		panic(NextHandlerNilError.Error())
	}
	return &peekLockRenewer{
		next:            next,
		lockRenewer:     lockrenewer,
		renewalInterval: interval,
	}
}

func (plr *peekLockRenewer) startPeriodicRenewal(ctx context.Context, message *servicebus.Message) {
	_, span := tracing.StartSpanFromMessageAndContext(ctx, "go-shuttle.peeklock.startPeriodicRenewal", message)
	defer span.End()
	for alive := true; alive; {
		select {
		case <-time.After(*plr.renewalInterval):
			span.Logger().Debug("Renewing message lock")
			err := plr.lockRenewer.RenewLocks(ctx, message)
			if err != nil {
				listener.Metrics.IncMessageLockRenewedFailure(message)
				// I don't think this is a problem. the context is canceled when the message processing is over.
				// this can happen if we already entered the interval case when the message is completing.
				span.Logger().Info("failed to renew the peek lock", tab.StringAttribute("reason", err.Error()))
				return
			}
			listener.Metrics.IncMessageLockRenewedSuccess(message)
		case <-ctx.Done():
			alive = false
			span.Logger().Info("Stopping periodic renewal")
			err := ctx.Err()
			if errors.Is(err, context.DeadlineExceeded) {
				listener.Metrics.IncMessageDeadlineReachedCount(message)
			}
		}
	}
}
