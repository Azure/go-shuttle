package peeklock

import (
	"context"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
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
	for alive := true; alive; {
		select {
		case <-time.After(interval):
			// add span for logs
			// fmt.Printf("%s - Renewing Lock: [ ttl: %s, lockedUntil: %s, enqueuedTime: %s ]",
			// 	time.Now(),
			// 	message.TTL,
			// 	message.SystemProperties.LockedUntil,
			// 	message.SystemProperties.EnqueuedTime)
			plr.lockRenewer.RenewLocks(renewerCtx, message)
			// what do we do if we fail to renew?
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
