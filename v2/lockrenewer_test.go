package shuttle_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	. "github.com/onsi/gomega"

	"github.com/Azure/go-shuttle/v2"
)

type fakeSBLockRenewer struct {
	RenewCount int
	Err        error
}

func (r *fakeSBLockRenewer) RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage,
	options *azservicebus.RenewMessageLockOptions) error {
	r.RenewCount++
	return r.Err
}

func Test_RenewPeriodically(t *testing.T) {
	renewer := &fakeSBLockRenewer{}
	interval := 50 * time.Millisecond
	lr := shuttle.NewRenewLockHandler(renewer, &interval, shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler,
		message *azservicebus.ReceivedMessage) {
		time.Sleep(150 * time.Millisecond)
	}))
	msg := &azservicebus.ReceivedMessage{}
	ctx, cancel := context.WithTimeout(context.TODO(), 120*time.Millisecond)
	defer cancel()
	lr.Handle(ctx, &fakeSettler{}, msg)
	g := NewWithT(t)
	g.Eventually(
		func(g Gomega) { g.Expect(renewer.RenewCount).To(Equal(2)) },
		130*time.Millisecond,
		20*time.Millisecond).Should(Succeed())
}

func Test_RenewPeriodically_Error(t *testing.T) {
	renewer := &fakeSBLockRenewer{
		Err: fmt.Errorf("fail to renew"),
	}
	interval := 50 * time.Millisecond
	lr := shuttle.NewRenewLockHandler(renewer, &interval, shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler,
		message *azservicebus.ReceivedMessage) {
		time.Sleep(150 * time.Millisecond)
	}))
	msg := &azservicebus.ReceivedMessage{}
	ctx, cancel := context.WithTimeout(context.TODO(), 120*time.Millisecond)
	defer cancel()
	lr.Handle(ctx, &fakeSettler{}, msg)

	g := NewWithT(t)
	// continue periodic renewal on Renew error
	g.Eventually(
		func(g Gomega) { g.Expect(renewer.RenewCount).To(Equal(2)) },
		130*time.Millisecond,
		20*time.Millisecond).Should(Succeed())
}

func Test_RenewPeriodically_ContextCanceled(t *testing.T) {
	renewer := &fakeSBLockRenewer{
		Err: fmt.Errorf("fail to renew"),
	}
	interval := 50 * time.Millisecond
	lr := shuttle.NewRenewLockHandler(renewer, &interval, shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler,
		message *azservicebus.ReceivedMessage) {
		time.Sleep(150 * time.Millisecond)
	}))
	msg := &azservicebus.ReceivedMessage{}
	ctx, cancel := context.WithCancel(context.TODO())
	cancel()
	lr.Handle(ctx, &fakeSettler{}, msg)

	g := NewWithT(t)
	// continue periodic renewal on Renew error
	g.Consistently(func() bool { return renewer.RenewCount == 0 }, 130*time.Millisecond, 20*time.Millisecond)
}
