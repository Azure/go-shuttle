package shuttle

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	. "github.com/onsi/gomega"
)

func TestFunc_NewSender(t *testing.T) {
	marshaller := &DefaultProtoMarshaller{}
	sender := NewSender(nil, &SenderOptions{Marshaller: marshaller})

	if sender.options.Marshaller != marshaller {
		t.Errorf("failed to set marshaller, expected: %s, actual: %s", reflect.TypeOf(marshaller), reflect.TypeOf(sender.options.Marshaller))
	}
}

func TestHandlers_SetMessageId(t *testing.T) {
	randId := "testmessageid"

	blankMsg := &azservicebus.Message{}
	handler := SetMessageId(&randId)
	if err := handler(blankMsg); err != nil {
		t.Errorf("Unexpected error in set message id test: %s", err)
	}
	if *blankMsg.MessageID != randId {
		t.Errorf("for message id expected %s, got %s", randId, *blankMsg.MessageID)
	}
}

func TestHandlers_SetCorrelationId(t *testing.T) {
	randId := "testcorrelationid"

	blankMsg := &azservicebus.Message{}
	handler := SetCorrelationId(&randId)
	if err := handler(blankMsg); err != nil {
		t.Errorf("Unexpected error in set correlation id test: %s", err)
	}
	if *blankMsg.CorrelationID != randId {
		t.Errorf("for correlation id expected %s, got %s", randId, *blankMsg.CorrelationID)
	}
}

func TestHandlers_SetScheduleAt(t *testing.T) {
	blankMsg := &azservicebus.Message{}
	currentTime := time.Now()
	handler := SetScheduleAt(currentTime)
	if err := handler(blankMsg); err != nil {
		t.Errorf("Unexpected error in set schedule at test: %s", err)
	}
	if *blankMsg.ScheduledEnqueueTime != currentTime {
		t.Errorf("for schedule at expected %s, got %s", currentTime, *blankMsg.ScheduledEnqueueTime)
	}
}

func TestHandlers_SetMessageDelay(t *testing.T) {
	blankMsg := &azservicebus.Message{}
	g := NewWithT(t)
	option := SetMessageDelay(1 * time.Minute)
	if err := option(blankMsg); err != nil {
		t.Errorf("Unexpected error in set schedule at test: %s", err)
	}
	g.Expect(*blankMsg.ScheduledEnqueueTime).To(BeTemporally("~", time.Now().Add(1*time.Minute), time.Second))
}

func TestHandlers_SetMessageTTL(t *testing.T) {
	blankMsg := &azservicebus.Message{}
	ttl := 10 * time.Second
	option := SetMessageTTL(ttl)
	if err := option(blankMsg); err != nil {
		t.Errorf("Unexpected error in set message TTL at test: %s", err)
	}
	if *blankMsg.TimeToLive != ttl {
		t.Errorf("for message TTL at expected %s, got %s", ttl, *blankMsg.TimeToLive)
	}
}

func TestSender_SenderTracePropagation(t *testing.T) {
	g := NewWithT(t)
	azSender := &fakeAzSender{}
	sender := NewSender(azSender, &SenderOptions{
		EnableTracingPropagation: true,
		Marshaller:               &DefaultJSONMarshaller{},
	})

	tp := trace.NewTracerProvider(trace.WithSampler(trace.AlwaysSample()))
	ctx, span := tp.Tracer("testTracer").Start(
		context.Background(),
		"receiver.Handle")

	msg, err := sender.ToServiceBusMessage(ctx, "test")
	span.End()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(msg.ApplicationProperties["traceparent"]).ToNot(BeNil())
}

func TestSender_WithDefaultSendTimeout(t *testing.T) {
	g := NewWithT(t)
	azSender := &fakeAzSender{
		DoSendMessage: func(ctx context.Context, message *azservicebus.Message, options *azservicebus.SendMessageOptions) error {
			dl, ok := ctx.Deadline()
			g.Expect(ok).To(BeTrue())
			g.Expect(dl).To(BeTemporally("~", time.Now().Add(defaultSendTimeout), time.Second))
			return nil
		},
		DoSendMessageBatch: func(ctx context.Context, messages *azservicebus.MessageBatch, options *azservicebus.SendMessageBatchOptions) error {
			dl, ok := ctx.Deadline()
			g.Expect(ok).To(BeTrue())
			g.Expect(dl).To(BeTemporally("~", time.Now().Add(defaultSendTimeout), time.Second))
			return nil
		},
	}
	sender := NewSender(azSender, &SenderOptions{
		Marshaller: &DefaultJSONMarshaller{},
	})
	err := sender.SendMessage(context.Background(), "test")
	g.Expect(err).ToNot(HaveOccurred())
	err = sender.SendMessageBatch(context.Background(), nil)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestSender_WithSendTimeout(t *testing.T) {
	g := NewWithT(t)
	sendTimeout := 5 * time.Second
	azSender := &fakeAzSender{
		DoSendMessage: func(ctx context.Context, message *azservicebus.Message, options *azservicebus.SendMessageOptions) error {
			dl, ok := ctx.Deadline()
			g.Expect(ok).To(BeTrue())
			g.Expect(dl).To(BeTemporally("~", time.Now().Add(sendTimeout), time.Second))
			return nil
		},
		DoSendMessageBatch: func(ctx context.Context, messages *azservicebus.MessageBatch, options *azservicebus.SendMessageBatchOptions) error {
			dl, ok := ctx.Deadline()
			g.Expect(ok).To(BeTrue())
			g.Expect(dl).To(BeTemporally("~", time.Now().Add(sendTimeout), time.Second))
			return nil
		},
	}
	sender := NewSender(azSender, &SenderOptions{
		Marshaller:  &DefaultJSONMarshaller{},
		SendTimeout: sendTimeout,
	})
	err := sender.SendMessage(context.Background(), "test")
	g.Expect(err).ToNot(HaveOccurred())
	err = sender.SendMessageBatch(context.Background(), nil)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestSender_WithContextCanceled(t *testing.T) {
	g := NewWithT(t)
	sendTimeout := 1 * time.Second
	azSender := &fakeAzSender{
		DoSendMessage: func(ctx context.Context, message *azservicebus.Message, options *azservicebus.SendMessageOptions) error {
			time.Sleep(2 * time.Second)
			return nil
		},
		DoSendMessageBatch: func(ctx context.Context, messages *azservicebus.MessageBatch, options *azservicebus.SendMessageBatchOptions) error {
			time.Sleep(2 * time.Second)
			return nil
		},
	}
	sender := NewSender(azSender, &SenderOptions{
		Marshaller:  &DefaultJSONMarshaller{},
		SendTimeout: sendTimeout,
	})

	err := sender.SendMessage(context.Background(), "test")
	g.Expect(err).To(MatchError(context.DeadlineExceeded))
	err = sender.SendMessageBatch(context.Background(), nil)
	g.Expect(err).To(MatchError(context.DeadlineExceeded))
}

func TestSender_SendWithCanceledContext(t *testing.T) {
	g := NewWithT(t)
	azSender := &fakeAzSender{
		DoSendMessage: func(ctx context.Context, message *azservicebus.Message, options *azservicebus.SendMessageOptions) error {
			return nil
		},
		DoSendMessageBatch: func(ctx context.Context, messages *azservicebus.MessageBatch, options *azservicebus.SendMessageBatchOptions) error {
			return nil
		},
	}
	sender := NewSender(azSender, &SenderOptions{
		Marshaller: &DefaultJSONMarshaller{},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sender.SendMessage(ctx, "test")
	g.Expect(err).To(MatchError(context.Canceled))
	err = sender.SendMessageBatch(ctx, nil)
	g.Expect(err).To(MatchError(context.Canceled))
}

func TestSender_DisabledSendTimeout(t *testing.T) {
	g := NewWithT(t)
	sendTimeout := -1 * time.Second
	azSender := &fakeAzSender{
		DoSendMessage: func(ctx context.Context, message *azservicebus.Message, options *azservicebus.SendMessageOptions) error {
			_, ok := ctx.Deadline()
			g.Expect(ok).To(BeFalse())
			return nil
		},
		DoSendMessageBatch: func(ctx context.Context, messages *azservicebus.MessageBatch, options *azservicebus.SendMessageBatchOptions) error {
			_, ok := ctx.Deadline()
			g.Expect(ok).To(BeFalse())
			return nil
		},
	}
	sender := NewSender(azSender, &SenderOptions{
		Marshaller:  &DefaultJSONMarshaller{},
		SendTimeout: sendTimeout,
	})
	err := sender.SendMessage(context.Background(), "test")
	g.Expect(err).ToNot(HaveOccurred())
	err = sender.SendMessageBatch(context.Background(), nil)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestSender_SendMessage(t *testing.T) {
	azSender := &fakeAzSender{}
	sender := NewSender(azSender, nil)
	err := sender.SendMessage(context.Background(), "test", SetMessageId(to.Ptr("messageID")))
	g := NewWithT(t)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(azSender.SendMessageCalled).To(BeTrue())
	g.Expect(string(azSender.SendMessageReceivedValue.Body)).To(Equal("\"test\""))
	g.Expect(*azSender.SendMessageReceivedValue.MessageID).To(Equal("messageID"))

	azSender.SendMessageErr = fmt.Errorf("msg send failure")
	err = sender.SendMessage(context.Background(), "test")
	g.Expect(err).To(And(HaveOccurred(), MatchError(azSender.SendMessageErr)))
}

func TestSender_SendMessageBatch(t *testing.T) {
	g := NewWithT(t)
	azSender := &fakeAzSender{
		NewMessageBatchReturnValue: &azservicebus.MessageBatch{},
	}
	sender := NewSender(azSender, nil)
	msg, err := sender.ToServiceBusMessage(context.Background(), "test")
	g.Expect(err).ToNot(HaveOccurred())
	err = sender.SendMessageBatch(context.Background(), []*azservicebus.Message{msg})
	g.Expect(err).To(HaveOccurred())
	// No way to create a MessageBatch struct with a non-0 max bytes in test, so the best we can do is expect an error.
}

func TestSender_SendMessageBatchWithContextCanceled(t *testing.T) {
	g := NewWithT(t)
	azSender := &fakeAzSender{}
	sender := NewSender(azSender, nil)

	// Create a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sender.SendMessageBatch(ctx, []*azservicebus.Message{})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(context.Canceled))
	g.Expect(azSender.SendMessageBatchCalled).To(BeFalse())
}

func TestSender_AzSender(t *testing.T) {
	g := NewWithT(t)
	azSender := &fakeAzSender{}
	sender := NewSender(azSender, nil)
	g.Expect(sender.AzSender()).To(Equal(azSender))
}

type fakeAzSender struct {
	DoSendMessage                 func(ctx context.Context, message *azservicebus.Message, options *azservicebus.SendMessageOptions) error
	DoSendMessageBatch            func(ctx context.Context, batch *azservicebus.MessageBatch, options *azservicebus.SendMessageBatchOptions) error
	SendMessageReceivedValue      *azservicebus.Message
	SendMessageReceivedCtx        context.Context
	SendMessageCalled             bool
	SendMessageErr                error
	SendMessageBatchCalled        bool
	SendMessageBatchErr           error
	NewMessageBatchReturnValue    *azservicebus.MessageBatch
	NewMessageBatchErr            error
	SendMessageBatchReceivedValue *azservicebus.MessageBatch
}

func (f *fakeAzSender) SendMessage(
	ctx context.Context,
	message *azservicebus.Message,
	options *azservicebus.SendMessageOptions) error {
	f.SendMessageCalled = true
	f.SendMessageReceivedValue = message
	f.SendMessageReceivedCtx = ctx
	if f.DoSendMessage != nil {
		if err := f.DoSendMessage(ctx, message, options); err != nil {
			return err
		}
	}
	return f.SendMessageErr
}

func (f *fakeAzSender) SendMessageBatch(
	ctx context.Context,
	batch *azservicebus.MessageBatch,
	options *azservicebus.SendMessageBatchOptions) error {
	f.SendMessageBatchCalled = true
	f.SendMessageBatchReceivedValue = batch
	if f.DoSendMessageBatch != nil {
		if err := f.DoSendMessageBatch(ctx, batch, options); err != nil {
			return err
		}
	}
	return f.SendMessageBatchErr
}

func (f *fakeAzSender) NewMessageBatch(
	ctx context.Context,
	options *azservicebus.MessageBatchOptions) (*azservicebus.MessageBatch, error) {
	return f.NewMessageBatchReturnValue, f.NewMessageBatchErr
}
