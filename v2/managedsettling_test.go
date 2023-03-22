package shuttle

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
)

type fakeSettler struct {
	abandoned         bool
	completed         bool
	conpleteErr       error
	deadlettered      bool
	deadletterOptions *azservicebus.DeadLetterOptions
	defered           bool
	lockRenewed       bool
}

func (f *fakeSettler) AbandonMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.AbandonMessageOptions) error {
	f.abandoned = true
	return nil
}

func (f *fakeSettler) CompleteMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.CompleteMessageOptions) error {
	f.completed = true
	return f.conpleteErr
}

func (f *fakeSettler) DeadLetterMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeadLetterOptions) error {
	f.deadlettered = true
	f.deadletterOptions = options
	return nil
}

func (f *fakeSettler) DeferMessage(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.DeferMessageOptions) error {
	f.defered = true
	return nil
}

func (f *fakeSettler) RenewMessageLock(ctx context.Context, message *azservicebus.ReceivedMessage, options *azservicebus.RenewMessageLockOptions) error {
	f.lockRenewed = true
	return nil
}

type hooks struct {
	onAbandonedCalled    bool
	onCompleteCalled     bool
	onDeadLetteredCalled bool
}

func TestManagedSettler_Handle(t *testing.T) {
	testCases := []struct {
		name            string
		settler         fakeSettler
		hooks           *hooks
		handlerResponse error
		msg             *azservicebus.ReceivedMessage
		expectation     func(*testing.T, *hooks, *fakeSettler)
	}{
		{
			name:            "complete when handler returns nil",
			hooks:           &hooks{},
			handlerResponse: nil,
			msg:             &azservicebus.ReceivedMessage{},
			expectation: func(t *testing.T, hooks *hooks, settler *fakeSettler) {
				require.True(t, settler.completed)
				require.False(t, settler.abandoned)
			},
		},
		{
			name:            "complete triggers complete hook",
			hooks:           &hooks{},
			handlerResponse: nil,
			msg:             &azservicebus.ReceivedMessage{},
			expectation: func(t *testing.T, hooks *hooks, settler *fakeSettler) {
				require.True(t, hooks.onCompleteCalled)
				require.False(t, hooks.onAbandonedCalled)
			},
		},
		{
			name:            "abandon when handler returns err",
			hooks:           &hooks{},
			handlerResponse: fmt.Errorf("some error"),
			msg:             &azservicebus.ReceivedMessage{},
			expectation: func(t *testing.T, hooks *hooks, settler *fakeSettler) {
				require.False(t, settler.completed)
				require.True(t, settler.abandoned)
			},
		},
		{
			name:            "abandon triggers abandon hook",
			hooks:           &hooks{},
			handlerResponse: fmt.Errorf("some error"),
			msg:             &azservicebus.ReceivedMessage{},
			expectation: func(t *testing.T, hooks *hooks, settler *fakeSettler) {
				require.True(t, hooks.onAbandonedCalled)
				require.False(t, hooks.onCompleteCalled)
			},
		},
		{
			name:            "complete returns error triggers abandon hook",
			hooks:           &hooks{},
			settler:         fakeSettler{conpleteErr: fmt.Errorf("failed to complete msg")},
			handlerResponse: nil, // handler succeeds
			msg:             &azservicebus.ReceivedMessage{},
			expectation: func(t *testing.T, hooks *hooks, settler *fakeSettler) {
				require.True(t, settler.completed)
				require.False(t, hooks.onCompleteCalled)
				require.True(t, hooks.onAbandonedCalled)
			},
		},
		{
			name:            "deadletter when handler returns err and retry decision is false",
			hooks:           &hooks{},
			handlerResponse: fmt.Errorf("some error"),
			msg:             &azservicebus.ReceivedMessage{DeliveryCount: 5},
			expectation: func(t *testing.T, hooks *hooks, settler *fakeSettler) {
				require.False(t, settler.completed)
				require.False(t, settler.abandoned)
				require.True(t, settler.deadlettered)
				require.True(t, hooks.onDeadLetteredCalled)
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(tt *testing.T) {
			options := &ManagedSettlingOptions{
				RetryDecision:      &MaxAttemptsRetryDecision{MaxAttempts: 5},
				RetryDelayStrategy: &ConstantDelayStrategy{Delay: 0},
				OnAbandoned: func(ctx context.Context, msg *azservicebus.ReceivedMessage, err error) {
					tc.hooks.onAbandonedCalled = true
				},
				OnDeadLettered: func(ctx context.Context, msg *azservicebus.ReceivedMessage, err error) {
					tc.hooks.onDeadLetteredCalled = true
				},
				OnCompleted: func(ctx context.Context, msg *azservicebus.ReceivedMessage) {
					tc.hooks.onCompleteCalled = true
				},
			}
			h := NewManagedSettlingHandler(options,
				func(ctx context.Context, message *azservicebus.ReceivedMessage) error {
					return tc.handlerResponse
				})
			h.Handle(context.TODO(), &tc.settler, tc.msg)
			tc.expectation(tt, tc.hooks, &tc.settler)
		})
	}
}

func Test_NilErr_WrappedInDeadLetter(t *testing.T) {
	h := NewManagedSettlingHandler(nil, nil)
	settler := &fakeSettler{}
	h.handleError(context.TODO(), settler, &azservicebus.ReceivedMessage{DeliveryCount: 6}, nil)
	g := NewWithT(t)
	g.Expect(*settler.deadletterOptions.ErrorDescription).To(HavePrefix("nil error:"))
}

func TestDefaultOptions_CallDefaultHooks(t *testing.T) {
	h := NewManagedSettlingHandler(&ManagedSettlingOptions{
		RetryDelayStrategy: &ConstantDelayStrategy{Delay: 0},
	}, func(_ context.Context, _ *azservicebus.ReceivedMessage) error {
		return nil
	})

	settler := &fakeSettler{}
	h.Handle(context.TODO(), settler, &azservicebus.ReceivedMessage{})
	g := NewWithT(t)
	g.Expect(settler.completed).To(BeTrue())

	settler = &fakeSettler{}
	h.handleError(context.TODO(), settler, &azservicebus.ReceivedMessage{DeliveryCount: 0}, fmt.Errorf("oops"))
	g.Expect(settler.abandoned).To(BeTrue())

	settler = &fakeSettler{}
	h.handleError(context.TODO(), settler, &azservicebus.ReceivedMessage{DeliveryCount: 6}, fmt.Errorf("oops"))
	g.Expect(settler.deadlettered).To(BeTrue())
	g.Expect(*settler.deadletterOptions.ErrorDescription).To(Equal("oops"))
}

func TestMaxAttemptsRetryDecision(t *testing.T) {
	for _, tc := range []struct {
		maxAttempts   uint32
		deliveryCount uint32
		canRetry      bool
	}{
		{maxAttempts: 0, deliveryCount: 0, canRetry: false},
		{maxAttempts: 1, deliveryCount: 0, canRetry: true},
		{maxAttempts: 0, deliveryCount: 1, canRetry: false},
		{maxAttempts: 5, deliveryCount: 4, canRetry: true},
		{maxAttempts: 5, deliveryCount: 5, canRetry: false},
		{maxAttempts: 5, deliveryCount: 6, canRetry: false},
	} {
		t.Run(fmt.Sprintf("max %d - delivery %d", tc.maxAttempts, tc.deliveryCount), func(t *testing.T) {
			d := MaxAttemptsRetryDecision{MaxAttempts: tc.maxAttempts}
			res := d.CanRetry(nil, &azservicebus.ReceivedMessage{DeliveryCount: tc.deliveryCount})
			require.Equal(t, tc.canRetry, res)
		})
	}

}
