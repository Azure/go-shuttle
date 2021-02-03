package handlers

import (
	"context"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/peeklock"
	"github.com/Azure/go-shuttle/tracing"
	"github.com/devigned/tab"
)

var _ servicebus.Handler = (*PeekLockRenewer)(nil)

// PeekLockRenewer starts a background goroutine that renews the message lock at the given interval until Stop() is called
// or until the passed in context is canceled.
// it is a pass through handler if the renewalInterval is nil
type PeekLockRenewer struct {
	next            servicebus.Handler
	lockRenewer     peeklock.LockRenewer
	renewalInterval *time.Duration
}

func (plr *PeekLockRenewer) Handle(ctx context.Context, msg *servicebus.Message) error {
	if plr.renewalInterval != nil {
		go plr.startPeriodicRenewal(ctx, msg)
	}
	if plr.next != nil {
		return plr.next.Handle(ctx, msg)
	}
	return nil
}

func NewPeekLockRenewer(next servicebus.Handler, interval *time.Duration, lockrenewer peeklock.LockRenewer) *PeekLockRenewer {
	return &PeekLockRenewer{
		next:            next,
		lockRenewer:     lockrenewer,
		renewalInterval: interval,
	}
}

func (plr *PeekLockRenewer) startPeriodicRenewal(ctx context.Context, message *servicebus.Message) {
	_, span := tracing.StartSpanFromMessageAndContext(ctx, "go-shuttle.peeklock.startPeriodicRenewal", message)
	defer span.End()
	for alive := true; alive; {
		select {
		case <-time.After(*plr.renewalInterval):
			span.Logger().Debug("Renewing message lock")
			err := plr.lockRenewer.RenewLocks(ctx, message)
			if err != nil {
				// I don't think this is not a problem. the context is canceled when the message processing is over.
				// this can happen if we already entered the interval case when the message is completing.
				span.Logger().Info("failed to renew the peek lock", tab.StringAttribute("reason", err.Error()))
			}
		case <-ctx.Done():
			span.Logger().Info("Stopping periodic renewal")
			alive = false
		}
	}
}
