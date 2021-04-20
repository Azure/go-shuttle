package message_test

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/message"
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

func getValidMessage() *servicebus.Message {
	msg := servicebus.NewMessageFromString("")
	msg.UserProperties = map[string]interface{}{"type": "SomethingHappened"}
	return msg
}

func TestDo_ErrorNoType(t *testing.T) {
	g := NewWithT(t)
	msg := &servicebus.Message{}
	h := completeHandler.Do(context.Background(), nil, msg)
	g.Expect(h).To(BeAssignableToTypeOf(message.Error(errors.New(""))))
}

func TestDo_Complete(t *testing.T) {
	g := NewWithT(t)
	h := completeHandler.Do(context.Background(), nil, getValidMessage())
	g.Expect(h).To(BeAssignableToTypeOf(message.Complete()))
}

func TestDo_RetryLaterSuccess(t *testing.T) {
	t.Skip("this test needs to be aligned with behavior")
	g := NewWithT(t)
	var tester = &retryTester{}
	var returnedHandler message.Handler
	go func() {
		returnedHandler = message.RetryLater(5*time.Millisecond).
			Do(context.TODO(), tester, getValidMessage())
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
	t.Skip("this test needs to be aligned with behavior")
	g := NewWithT(t)
	var tester = &retryTester{}
	var returnedHandler message.Handler
	canceledCtx, cancel := context.WithCancel(context.TODO())
	cancel()
	returnedHandler = message.RetryLater(5*time.Millisecond).
		Do(canceledCtx, tester, getValidMessage())
	// verify that the retrylater handler does not return right away
	g.Expect(returnedHandler).To(BeAssignableToTypeOf(message.Error(context.Canceled)))
}
