package shuttle_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
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

func Test_StopRenewingOnHandlerCompletion(t *testing.T) {
	renewer := &fakeSBLockRenewer{}
	settler := &fakeSettler{}
	g := NewWithT(t)
	interval := 100 * time.Millisecond
	lr := shuttle.NewLockRenewalHandler(renewer, &shuttle.LockRenewalOptions{Interval: &interval},
		shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler,
			message *azservicebus.ReceivedMessage) {
			err := settler.CompleteMessage(ctx, message, nil)
			g.Expect(err).To(Not(HaveOccurred()))
		}))
	msg := &azservicebus.ReceivedMessage{}
	ctx, cancel := context.WithTimeout(context.TODO(), 120*time.Millisecond)
	defer cancel()
	lr.Handle(ctx, settler, msg)
	g.Expect(settler.CompleteCalled.Load()).To(Equal(int32(1)))
	g.Consistently(
		func(g Gomega) { g.Expect(renewer.RenewCount.Load()).To(Equal(int32(0))) },
		130*time.Millisecond,
		20*time.Millisecond).Should(Succeed())
}

func Test_RenewPeriodically(t *testing.T) {
	renewer := &fakeSBLockRenewer{}
	interval := 50 * time.Millisecond
	lr := shuttle.NewLockRenewalHandler(renewer, &shuttle.LockRenewalOptions{Interval: &interval},
		shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler,
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

//nolint:staticcheck // still need to cover the deprecated func
func Test_NewLockRenewerHandler_defaultToNotCancelMessageContext(t *testing.T) {
	g := NewWithT(t)
	interval := 20 * time.Millisecond
	sbRenewer := &fakeSBLockRenewer{
		Err: &azservicebus.Error{Code: azservicebus.CodeLockLost},
	}

	handler := shuttle.NewRenewLockHandler(sbRenewer, &interval,
		shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler, message *azservicebus.ReceivedMessage) {
			g.Consistently(func(g Gomega) {
				g.Expect(ctx.Err()).To(BeNil())
			}, "120ms", "10ms").Should(Succeed())
		}))
	handler.Handle(context.Background(), &fakeSettler{}, &azservicebus.ReceivedMessage{})
}

func Test_RenewPeriodically_Error(t *testing.T) {
	type testCase struct {
		name              string
		renewer           *fakeSBLockRenewer
		isRenewerCanceled bool
		cancelCtxOnStop   *bool
		gotMessageCtx     context.Context
		verify            func(g Gomega, tc *testCase)
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
			name:              "stop periodic renewal on context canceled",
			isRenewerCanceled: true,
			renewer:           &fakeSBLockRenewer{Err: context.Canceled},
			verify: func(g Gomega, tc *testCase) {
				g.Consistently(
					func(g Gomega) { g.Expect(tc.renewer.RenewCount.Load()).To(Equal(int32(0))) },
					130*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
			},
		},
		{
			name:    "stop periodic renewal on permanent error (lockLost)",
			renewer: &fakeSBLockRenewer{Err: &azservicebus.Error{Code: azservicebus.CodeLockLost}},
			verify: func(g Gomega, tc *testCase) {
				g.Consistently(
					func(g Gomega) { g.Expect(tc.renewer.RenewCount.Load()).To(Equal(int32(1))) },
					130*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
			},
		},
		{
			name:    "cancel message context on stop by default",
			renewer: &fakeSBLockRenewer{Err: &azservicebus.Error{Code: azservicebus.CodeLockLost}},
			verify: func(g Gomega, tc *testCase) {
				g.Consistently(
					func(g Gomega) { g.Expect(tc.renewer.RenewCount.Load()).To(Equal(int32(1))) },
					130*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
				g.Expect(tc.gotMessageCtx.Err()).To(Equal(context.Canceled))
			},
		},
		{
			name:            "does not cancel message context on stop if disabled",
			renewer:         &fakeSBLockRenewer{Err: &azservicebus.Error{Code: azservicebus.CodeLockLost}},
			cancelCtxOnStop: to.Ptr(false),
			verify: func(g Gomega, tc *testCase) {
				g.Consistently(
					func(g Gomega) {
						g.Expect(tc.renewer.RenewCount.Load()).To(Equal(int32(1)))
						g.Expect(tc.gotMessageCtx.Err()).To(BeNil())
					},
					100*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
			},
		},
		{
			name:    "continue periodic renewal on transient error (timeout)",
			renewer: &fakeSBLockRenewer{Err: &azservicebus.Error{Code: azservicebus.CodeTimeout}},
			verify: func(g Gomega, tc *testCase) {
				g.Eventually(
					func(g Gomega) { g.Expect(tc.renewer.RenewCount.Load()).To(Equal(int32(2))) },
					140*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			interval := 50 * time.Millisecond
			lr := shuttle.NewLockRenewalHandler(tc.renewer, &shuttle.LockRenewalOptions{Interval: &interval, CancelMessageContextOnStop: tc.cancelCtxOnStop},
				shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler,
					message *azservicebus.ReceivedMessage) {
					tc.gotMessageCtx = ctx
					select {
					case <-time.After(110 * time.Millisecond):
						break
					case <-ctx.Done():
						break
					}
				}))
			msg := &azservicebus.ReceivedMessage{}
			ctx, cancel := context.WithTimeout(context.TODO(), 200*time.Millisecond)
			if tc.isRenewerCanceled {
				cancel()
			}
			defer cancel()
			lr.Handle(ctx, &fakeSettler{}, msg)
			tc.verify(NewWithT(t), &tc)
		})
	}
}
