package peeklock_test

import (
	"context"
	"testing"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/peeklock"
	"github.com/stretchr/testify/assert"
)

type testLockRenewer struct {
	RenewCount int
}

func (t *testLockRenewer) RenewLocks(ctx context.Context, messages ...*servicebus.Message) error {
	t.RenewCount++
	return nil
}

var _ peeklock.LockRenewer = &testLockRenewer{}

func Test_RenewPeriodically(t *testing.T) {
	renewer := &testLockRenewer{}
	msg := &servicebus.Message{}
	p := peeklock.RenewPeriodically(context.TODO(), 50*time.Millisecond, renewer, msg)
	if !assert.Eventually(t, func() bool { return renewer.RenewCount == 4 }, 220*time.Millisecond, 10*time.Millisecond) {
		t.Errorf("renewed %d times but expected 4", renewer.RenewCount)
	}
	p.Stop()
}

func Test_RenewPeriodicallyStopped(t *testing.T) {
	renewer := &testLockRenewer{}
	msg := &servicebus.Message{}
	p := peeklock.RenewPeriodically(context.TODO(), 50*time.Millisecond, renewer, msg)
	p.Stop()
	if !assert.Eventually(t, func() bool { return renewer.RenewCount == 0 }, 100*time.Millisecond, 10*time.Millisecond) {
		t.Errorf("should not renew as we stop it early")
	}
}

func Test_RenewPeriodically_ParentContextCanceled(t *testing.T) {
	renewer := &testLockRenewer{}
	msg := &servicebus.Message{}
	ctx, cancel := context.WithCancel(context.TODO())
	peeklock.RenewPeriodically(ctx, 50*time.Millisecond, renewer, msg)
	cancel()
	if !assert.Eventually(t, func() bool { return renewer.RenewCount == 0 }, 100*time.Millisecond, 10*time.Millisecond) {
		t.Errorf("should not renew as we canceled the parent context it early")
	}
}
