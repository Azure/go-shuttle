package peeklock

import (
	"context"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/devigned/tab"
)

type LockRenewer interface {
	RenewLocks(ctx context.Context, messages ...*servicebus.Message) error
}

type PeriodicLockRenewer struct {
	lockRenewer LockRenewer
	cancelFunc  func()
	timer       *time.Timer
}

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
	_, span := tab.StartSpan(renewerCtx, "go-shuttle.peeklock.startPeriodicRenewal")
	defer span.End()
	for alive := true; alive; {
		select {
		case <-time.After(interval):
			// TODO: improve span once tracing package extracted
			span.Logger().Debug("Renewing message lock",
				tab.StringAttribute("message.TTL", message.TTL.String()),
				tab.StringAttribute("message.LockedUntil", message.SystemProperties.LockedUntil.String()),
				tab.StringAttribute("message.EnqueuedTime", message.SystemProperties.EnqueuedTime.String()))
			plr.lockRenewer.RenewLocks(renewerCtx, message)
		case <-renewerCtx.Done():
			// stop renewing
			alive = false
		}
	}
}

func (plr *PeriodicLockRenewer) Stop() {
	if plr.cancelFunc != nil {
		plr.cancelFunc()
	}
}
