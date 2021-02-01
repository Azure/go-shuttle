package peeklock

import (
	"context"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/tracing"
)

type LockRenewer interface {
	RenewLocks(ctx context.Context, messages ...*servicebus.Message) error
}

type PeriodicLockRenewer struct {
	lockRenewer LockRenewer
	cancelFunc  func()
	timer       *time.Timer
}

// RenewPeriodically starts a background goroutine that renews the message lock at the given interval until Stop() is called
// or until the passed in context is canceled.
func RenewPeriodically(ctx context.Context, interval time.Duration, lockrenewer LockRenewer, msg *servicebus.Message) *PeriodicLockRenewer {
	p := &PeriodicLockRenewer{
		lockRenewer: lockrenewer,
	}
	go p.startPeriodicRenewal(ctx, interval, msg)
	return p
}

func (plr *PeriodicLockRenewer) startPeriodicRenewal(ctx context.Context, interval time.Duration, message *servicebus.Message) {
	var renewerCtx context.Context
	renewerCtx, plr.cancelFunc = context.WithCancel(ctx)
	_, span := tracing.StartSpanFromMessageAndContext(renewerCtx, "go-shuttle.peeklock.startPeriodicRenewal", message)
	defer span.End()
	for alive := true; alive; {
		select {
		case <-time.After(interval):
			span.Logger().Debug("Renewing message lock")
			err := plr.lockRenewer.RenewLocks(renewerCtx, message)
			span.Logger().Error(err)
		case <-renewerCtx.Done():
			span.Logger().Info("Stopping periodic renewal")
			alive = false
		}
	}
}

func (plr *PeriodicLockRenewer) Stop() {
	if plr.cancelFunc != nil {
		plr.cancelFunc()
	}
}
