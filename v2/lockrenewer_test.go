package shuttle_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	. "github.com/onsi/gomega"

	"github.com/Azure/go-shuttle/v2"
)

type fakeSBLockRenewer struct {
	RenewCount atomic.Int32
	Err        error
}

func (r *fakeSBLockRenewer) RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage,
	options *azservicebus.RenewMessageLockOptions) error {
	r.RenewCount.Add(1)
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
		func(g Gomega) { g.Expect(renewer.RenewCount.Load()).To(Equal(int32(2))) },
		130*time.Millisecond,
		20*time.Millisecond).Should(Succeed())
}

func Test_RenewPeriodically_Error(t *testing.T) {
	type testCase struct {
		name    string
		renewer *fakeSBLockRenewer
		verify  func(g Gomega, tc *testCase)
	}
	testCases := []testCase{
		{
			name:    "continue periodic renewal on unknown error",
			renewer: &fakeSBLockRenewer{Err: fmt.Errorf("unknown error")},
			verify: func(g Gomega, tc *testCase) {
				g.Eventually(
					func(g Gomega) { g.Expect(tc.renewer.RenewCount.Load()).To(Equal(int32(2))) },
					130*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
			},
		},
		{
			name:    "stop periodic renewal on context canceled",
			renewer: &fakeSBLockRenewer{Err: context.Canceled},
			verify: func(g Gomega, tc *testCase) {
				g.Consistently(
					func(g Gomega) { g.Expect(tc.renewer.RenewCount.Load()).To(Equal(int32(2))) },
					130*time.Millisecond,
					20*time.Millisecond)
			},
		},
		{
			name:    "stop periodic renewal on permanent error (lockLost)",
			renewer: &fakeSBLockRenewer{Err: &azservicebus.Error{Code: azservicebus.CodeLockLost}},
			verify: func(g Gomega, tc *testCase) {
				g.Consistently(
					func(g Gomega) { g.Expect(tc.renewer.RenewCount.Load()).To(Equal(int32(2))) },
					130*time.Millisecond,
					20*time.Millisecond)
			},
		},
		{
			name:    "continue periodic renewal on transient error (timeout)",
			renewer: &fakeSBLockRenewer{Err: &azservicebus.Error{Code: azservicebus.CodeTimeout}},
			verify: func(g Gomega, tc *testCase) {
				g.Eventually(
					func(g Gomega) { g.Expect(tc.renewer.RenewCount.Load()).To(Equal(int32(2))) },
					130*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			interval := 50 * time.Millisecond
			lr := shuttle.NewRenewLockHandler(tc.renewer, &interval, shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler,
				message *azservicebus.ReceivedMessage) {
				time.Sleep(150 * time.Millisecond)
			}))
			msg := &azservicebus.ReceivedMessage{}
			ctx, cancel := context.WithTimeout(context.TODO(), 120*time.Millisecond)
			defer cancel()
			lr.Handle(ctx, &fakeSettler{}, msg)
			tc.verify(NewWithT(t), &tc)
		})
	}
}
