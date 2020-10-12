package message_test

import (
	"context"
	"errors"
	"testing"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/keikumata/azure-pub-sub/message"
	. "github.com/onsi/gomega"
)

var completeHandler = message.HandleFunc(func(ctx context.Context, m *message.Message) message.Handler {
	return m.Complete()
})

type retryTester struct {
	Count int
}

func (h *retryTester) Do(_ context.Context, _ message.Handler, m *servicebus.Message) message.Handler {
	return message.Complete()
}

var validMessage = &servicebus.Message{
	UserProperties: map[string]interface{}{"type": "SomethingHappened"},
}

func TestDo_ErrorNoType(t *testing.T) {
	g := NewWithT(t)
	msg := &servicebus.Message{}
	h := completeHandler.Do(context.Background(), nil, msg)
	g.Expect(h).To(BeAssignableToTypeOf(message.Error(errors.New(""))))
}

func TestDo_Complete(t *testing.T) {
	g := NewWithT(t)
	h := completeHandler.Do(context.Background(), nil, validMessage)
	g.Expect(h).To(BeAssignableToTypeOf(message.Complete()))
}

func TestDo_RetryLaterSuccess(t *testing.T) {
	g := NewWithT(t)
	var tester = &retryTester{}
	var returnedHandler message.Handler
	go func() {
		returnedHandler = message.RetryLater(5*time.Millisecond).
			Do(context.TODO(), tester, validMessage)
	}()
	// verify that the retrylater handler does not return right away
	g.Expect(returnedHandler).To(BeNil())
	// eventually, it does
	g.Eventually(func() message.Handler {
		return returnedHandler
	}, 10*time.Millisecond, time.Millisecond).ShouldNot(BeNil())
	// and gives you back your original handler to be run in the listener loop :)
	g.Expect(returnedHandler).To(Equal(tester))
}

func TestDo_RetryLaterContextCanceled(t *testing.T) {
	g := NewWithT(t)
	var tester = &retryTester{}
	var returnedHandler message.Handler
	canceledCtx, cancel := context.WithCancel(context.TODO())
	cancel()
	returnedHandler = message.RetryLater(5*time.Millisecond).
		Do(canceledCtx, tester, validMessage)
	// verify that the retrylater handler does not return right away
	g.Expect(returnedHandler).To(BeAssignableToTypeOf(message.Error(context.Canceled)))
}
