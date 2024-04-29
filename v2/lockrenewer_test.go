package shuttle_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Azure/go-shuttle/v2"
	"github.com/Azure/go-shuttle/v2/metrics/processor"
)

type fakeSBRenewLockSettler struct {
	fakeSettler

	PerMessage map[*azservicebus.ReceivedMessage]*atomic.Int32
	mapLock    sync.Mutex
	Err        error
}

func (r *fakeSBRenewLockSettler) RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage,
	_ *azservicebus.RenewMessageLockOptions) error {
	r.RenewCalled.Add(1)
	r.mapLock.Lock()
	defer r.mapLock.Unlock()
	if r.PerMessage == nil {
		r.PerMessage = map[*azservicebus.ReceivedMessage]*atomic.Int32{
			message: {},
		}
	}
	perMessageCount := r.PerMessage[message]
	if perMessageCount == nil {
		r.PerMessage[message] = &atomic.Int32{}
	}
	perMessageCount = r.PerMessage[message]
	perMessageCount.Add(1)
	return r.Err
}

func Test_StopRenewingOnHandlerCompletion(t *testing.T) {
	settler := &fakeSBRenewLockSettler{}
	g := NewWithT(t)
	interval := 100 * time.Millisecond
	lr := shuttle.NewRenewLockHandler(&shuttle.LockRenewalOptions{Interval: &interval},
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
		func(g Gomega) { g.Expect(settler.RenewCalled.Load()).To(Equal(int32(0))) },
		130*time.Millisecond,
		20*time.Millisecond).Should(Succeed())
}

func Test_RenewalHandlerStayIndependentPerMessage(t *testing.T) {
	settler := &fakeSBRenewLockSettler{}
	g := NewWithT(t)
	interval := 50 * time.Millisecond
	lr := shuttle.NewRenewLockHandler(&shuttle.LockRenewalOptions{Interval: &interval},
		shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler,
			message *azservicebus.ReceivedMessage) {
			// Sleep > 100ms to allow 2 renewal to happen
			time.Sleep(120 * time.Millisecond)
			err := settler.CompleteMessage(ctx, message, nil)
			g.Expect(err).To(Not(HaveOccurred()))
		}))
	// send 2 message with different context, cancel the 2nd context right away.
	// The 2nd message should not be renewed.
	// The 1st message should be renewed exactly twice
	msg1 := &azservicebus.ReceivedMessage{}
	msg2 := &azservicebus.ReceivedMessage{}
	ctx := context.Background()
	ctx1, cancel1 := context.WithCancel(ctx)
	ctx2, cancel2 := context.WithCancel(ctx)
	defer cancel1()
	defer cancel2()
	go lr.Handle(ctx1, settler, msg1)
	go lr.Handle(ctx2, settler, msg2)
	// cancel 2nd message context right away
	cancel2()
	g.Eventually(
		func(g Gomega) {
			g.Expect(settler.PerMessage[msg2]).To(BeNil(), "msg2 should not be in the map")
			g.Expect(settler.PerMessage[msg1]).ToNot(BeNil(), "msg1 should be in the map")
			g.Expect(settler.PerMessage[msg1].Load()).To(Equal(int32(2)))
		},
		200*time.Millisecond,
		10*time.Millisecond).Should(Succeed())
	g.Eventually(
		func(g Gomega) {
			g.Expect(settler.CompleteCalled.Load()).To(Equal(int32(2)))
		},
		100*time.Millisecond,
		10*time.Millisecond).Should(Succeed())

}

func Test_RenewPeriodically(t *testing.T) {
	settler := &fakeSBRenewLockSettler{}
	interval := 50 * time.Millisecond
	lr := shuttle.NewRenewLockHandler(&shuttle.LockRenewalOptions{Interval: &interval},
		shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler,
			message *azservicebus.ReceivedMessage) {
			time.Sleep(150 * time.Millisecond)
		}))
	msg := &azservicebus.ReceivedMessage{}
	ctx, cancel := context.WithTimeout(context.TODO(), 120*time.Millisecond)
	defer cancel()
	lr.Handle(ctx, settler, msg)
	g := NewWithT(t)
	g.Eventually(
		func(g Gomega) { g.Expect(settler.RenewCalled.Load()).To(Equal(int32(2))) },
		130*time.Millisecond,
		20*time.Millisecond).Should(Succeed())
}

//nolint:staticcheck // still need to cover the deprecated func
func Test_NewLockRenewalHandler_RenewPeriodically(t *testing.T) {
	settler := &fakeSBRenewLockSettler{}
	interval := 50 * time.Millisecond
	lr := shuttle.NewLockRenewalHandler(settler, &shuttle.LockRenewalOptions{Interval: &interval},
		shuttle.HandlerFunc(func(ctx context.Context, settler shuttle.MessageSettler,
			message *azservicebus.ReceivedMessage) {
			time.Sleep(150 * time.Millisecond)
		}))
	msg := &azservicebus.ReceivedMessage{}
	ctx, cancel := context.WithTimeout(context.TODO(), 120*time.Millisecond)
	defer cancel()
	lr.Handle(ctx, settler, msg)
	g := NewWithT(t)
	g.Eventually(
		func(g Gomega) { g.Expect(settler.RenewCalled.Load()).To(Equal(int32(2))) },
		130*time.Millisecond,
		20*time.Millisecond).Should(Succeed())
}

func Test_RenewPeriodically_Error(t *testing.T) {
	type testCase struct {
		name              string
		settler           *fakeSBRenewLockSettler
		isRenewerCanceled bool
		cancelCtxOnStop   *bool
		gotMessageCtx     context.Context
		verify            func(g Gomega, tc *testCase, metrics *processor.Informer)
	}
	testCases := []testCase{
		{
			name:    "continue periodic renewal on unknown error",
			settler: &fakeSBRenewLockSettler{Err: fmt.Errorf("unknown error")},
			verify: func(g Gomega, tc *testCase, metrics *processor.Informer) {
				g.Eventually(
					func(g Gomega) { g.Expect(tc.settler.RenewCalled.Load()).To(Equal(int32(2))) },
					130*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
			},
		},
		{
			name:              "stop periodic renewal on context canceled",
			isRenewerCanceled: false,
			settler:           &fakeSBRenewLockSettler{Err: context.Canceled},
			verify: func(g Gomega, tc *testCase, metrics *processor.Informer) {
				g.Consistently(
					func(g Gomega) {
						g.Expect(tc.settler.RenewCalled.Load()).To(Equal(int32(1)),
							"should not attempt to renew")
						g.Expect(metrics.GetMessageLockRenewedFailureCount()).To(Equal(float64(0)),
							"should not record failure metric")
					},
					130*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
			},
		},
		{
			name:              "stop periodic renewal on context canceled",
			isRenewerCanceled: true,
			settler:           &fakeSBRenewLockSettler{Err: context.Canceled},
			verify: func(g Gomega, tc *testCase, metrics *processor.Informer) {
				g.Consistently(
					func(g Gomega) { g.Expect(tc.settler.RenewCalled.Load()).To(Equal(int32(0))) },
					130*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
			},
		},
		{
			name:    "stop periodic renewal on permanent error (lockLost)",
			settler: &fakeSBRenewLockSettler{Err: &azservicebus.Error{Code: azservicebus.CodeLockLost}},
			verify: func(g Gomega, tc *testCase, metrics *processor.Informer) {
				g.Consistently(
					func(g Gomega) { g.Expect(tc.settler.RenewCalled.Load()).To(Equal(int32(1))) },
					130*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
			},
		},
		{
			name:    "cancel message context on stop by default",
			settler: &fakeSBRenewLockSettler{Err: &azservicebus.Error{Code: azservicebus.CodeLockLost}},
			verify: func(g Gomega, tc *testCase, metrics *processor.Informer) {
				g.Consistently(
					func(g Gomega) { g.Expect(tc.settler.RenewCalled.Load()).To(Equal(int32(1))) },
					130*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
				g.Expect(tc.gotMessageCtx.Err()).To(Equal(context.Canceled))
			},
		},
		{
			name:            "does not cancel message context on stop if disabled",
			settler:         &fakeSBRenewLockSettler{Err: &azservicebus.Error{Code: azservicebus.CodeLockLost}},
			cancelCtxOnStop: to.Ptr(false),
			verify: func(g Gomega, tc *testCase, metrics *processor.Informer) {
				g.Consistently(
					func(g Gomega) {
						g.Expect(tc.settler.RenewCalled.Load()).To(Equal(int32(1)))
						g.Expect(tc.gotMessageCtx.Err()).To(BeNil())
					},
					100*time.Millisecond,
					20*time.Millisecond).Should(Succeed())
			},
		},
		{
			name:    "continue periodic renewal on transient error (timeout)",
			settler: &fakeSBRenewLockSettler{Err: &azservicebus.Error{Code: azservicebus.CodeTimeout}},
			verify: func(g Gomega, tc *testCase, metrics *processor.Informer) {
				g.Eventually(
					func(g Gomega) { g.Expect(tc.settler.RenewCalled.Load()).To(Equal(int32(2))) },
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
			reg := processor.NewRegistry()
			reg.Init(prometheus.NewRegistry())
			informer := processor.NewInformerFor(reg)
			lr := shuttle.NewRenewLockHandler(
				&shuttle.LockRenewalOptions{
					Interval:                   &interval,
					CancelMessageContextOnStop: tc.cancelCtxOnStop,
					MetricRecorder:             reg,
				},
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
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			if tc.isRenewerCanceled {
				cancel()
			}
			defer cancel()
			lr.Handle(ctx, tc.settler, msg)
			tc.verify(NewWithT(t), &tc, informer)
		})
	}
}
