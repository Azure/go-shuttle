package shuttle

import (
	"context"
	"fmt"
	"reflect"
	"sync"
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

	sender = NewSender(nil, &SenderOptions{EnableTracingPropagation: true})
	if sender.options.Marshaller == nil {
		t.Errorf("failed to set marshaller")
	}
	if !sender.options.EnableTracingPropagation {
		t.Errorf("failed to set EnableTracingPropagation, expected: true, actual: %t", sender.options.EnableTracingPropagation)
	}
	if sender.options.SendTimeout != defaultSendTimeout {
		t.Errorf("failed to set SendTimeout, expected: %s, actual: %s", defaultSendTimeout, sender.options.SendTimeout)
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

func TestHandlers_SetSubject(t *testing.T) {
	subject := "test-subject"
	blankMsg := &azservicebus.Message{}
	handler := SetSubject(subject)
	if err := handler(blankMsg); err != nil {
		t.Errorf("Unexpected error in set subject test: %s", err)
	}
	if *blankMsg.Subject != subject {
		t.Errorf("for subject expected %s, got %s", subject, *blankMsg.Subject)
	}
}

func TestHandlers_SetTo(t *testing.T) {
	to := "test-to"
	blankMsg := &azservicebus.Message{}
	handler := SetTo(to)
	if err := handler(blankMsg); err != nil {
		t.Errorf("Unexpected error in set to test: %s", err)
	}
	if *blankMsg.To != to {
		t.Errorf("for to expected %s, got %s", to, *blankMsg.To)
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
	err = sender.SendMessageBatch(context.Background(), []*azservicebus.Message{})
	g.Expect(err).To(HaveOccurred())
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
	err = sender.SendMessageBatch(context.Background(), []*azservicebus.Message{})
	g.Expect(err).To(HaveOccurred())
	err = sender.SendAsBatch(context.Background(), []*azservicebus.Message{}, nil)
	g.Expect(err).To(HaveOccurred())
	err = sender.SendAsBatch(context.Background(), []*azservicebus.Message{}, &SendAsBatchOptions{AllowMultipleBatch: true})
	g.Expect(err).To(HaveOccurred())
}

func TestSender_WithContextCanceled(t *testing.T) {
	g := NewWithT(t)
	sendTimeout := 1 * time.Second
	azSender := &fakeAzSender{
		NewMessageBatchReturnValue: &azservicebus.MessageBatch{},
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
	err = sender.SendMessageBatch(context.Background(), []*azservicebus.Message{})
	g.Expect(err).To(HaveOccurred()) // Now expects error for empty messages instead of timeout
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
	err = sender.SendMessageBatch(ctx, []*azservicebus.Message{})
	g.Expect(err).To(HaveOccurred()) // Now expects error for empty messages instead of context canceled
}

func TestSender_DisabledSendTimeout(t *testing.T) {
	g := NewWithT(t)
	sendTimeout := -1 * time.Second
	azSender := &fakeAzSender{
		NewMessageBatchReturnValue: &azservicebus.MessageBatch{},
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
	err = sender.SendMessageBatch(context.Background(), []*azservicebus.Message{})
	g.Expect(err).To(HaveOccurred())
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

func TestSender_AzSender(t *testing.T) {
	g := NewWithT(t)
	azSender := &fakeAzSender{}
	sender := NewSender(azSender, nil)
	g.Expect(sender.AzSender()).To(Equal(azSender))
}

func TestSender_SetAzSender(t *testing.T) {
	g := NewWithT(t)
	azSender := &fakeAzSender{SendMessageErr: fmt.Errorf("msg send failure")}
	sender := NewSender(azSender, nil)
	err := sender.SendMessage(context.Background(), "test")
	g.Expect(err).To(HaveOccurred())
	sender.SetAzSender(&fakeAzSender{})
	err = sender.SendMessage(context.Background(), "test")
	g.Expect(err).ToNot(HaveOccurred())
	// coverage for deprecated FailOver() function
	sender.FailOver(&fakeAzSender{})
}

func TestSender_ConcurrentSendAndSetAzSender(t *testing.T) {
	g := NewWithT(t)
	azSender1 := &fakeAzSender{}
	azSender2 := &fakeAzSender{}
	sender := NewSender(azSender1, nil)

	var wg sync.WaitGroup
	numRoutines := 10

	// start multiple goroutines to send messages
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := sender.SendMessage(context.Background(), fmt.Sprintf("test%d", i))
			g.Expect(err).ToNot(HaveOccurred())
			time.Sleep(time.Duration(i) * time.Millisecond)
			err = sender.SendMessage(context.Background(), fmt.Sprintf("test%d-after-sleep", i))
			g.Expect(err).ToNot(HaveOccurred())
		}(i)
	}
	// call SetAzSender in the middle
	time.Sleep(5 * time.Millisecond)
	sender.SetAzSender(azSender2)
	// wait for all goroutines to finish
	wg.Wait()
	// check that some messages were sent with the first sender and some with the second
	g.Expect(azSender1.SendMessageCalled).To(BeTrue())
	g.Expect(azSender2.SendMessageCalled).To(BeTrue())
}

func TestSender_SendAsBatch_EmptyMessages_AllowMultiple(t *testing.T) {
	g := NewWithT(t)
	azSender := &fakeAzSender{}
	sender := NewSender(azSender, nil)

	options := &SendAsBatchOptions{AllowMultipleBatch: true}
	err := sender.SendAsBatch(context.Background(), []*azservicebus.Message{}, options)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cannot send empty message array"))
	// Should not call send since error returned early
	g.Expect(azSender.SendMessageBatchCalled).To(BeFalse())
	// No batches should be created
	g.Expect(azSender.BatchesCreated).To(Equal(0))
}

func TestSender_SendAsBatch_EmptyMessages_SingleBatch(t *testing.T) {
	g := NewWithT(t)
	azSender := &fakeAzSender{
		NewMessageBatchReturnValue: &azservicebus.MessageBatch{},
	}
	sender := NewSender(azSender, nil)

	options := &SendAsBatchOptions{AllowMultipleBatch: false}
	err := sender.SendAsBatch(context.Background(), []*azservicebus.Message{}, options)
	// Should fail because empty message array is not allowed
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cannot send empty message array"))
	// Should not attempt to send
	g.Expect(azSender.SendMessageBatchCalled).To(BeFalse())
	// No batch should be created
	g.Expect(azSender.BatchesCreated).To(Equal(0))
}

func TestSender_SendAsBatch_ContextCanceled(t *testing.T) {
	g := NewWithT(t)
	azSender := &fakeAzSender{}
	sender := NewSender(azSender, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msg, err := sender.ToServiceBusMessage(context.Background(), "test")
	g.Expect(err).ToNot(HaveOccurred())

	options := &SendAsBatchOptions{AllowMultipleBatch: true}
	err = sender.SendAsBatch(ctx, []*azservicebus.Message{msg}, options)
	g.Expect(err).To(MatchError(context.Canceled))
}

func TestSender_SendAsBatch_NewMessageBatchError(t *testing.T) {
	g := NewWithT(t)
	expectedErr := fmt.Errorf("batch creation failed")
	azSender := &fakeAzSender{
		NewMessageBatchErr: expectedErr,
	}
	sender := NewSender(azSender, nil)

	msg, err := sender.ToServiceBusMessage(context.Background(), "test")
	g.Expect(err).ToNot(HaveOccurred())

	options := &SendAsBatchOptions{AllowMultipleBatch: true}
	err = sender.SendAsBatch(context.Background(), []*azservicebus.Message{msg}, options)
	g.Expect(err).To(Equal(expectedErr))
	g.Expect(azSender.BatchesCreated).To(Equal(1))
	g.Expect(azSender.SendMessageBatchCalled).To(BeFalse()) // Should not try to send if batch creation fails
}

func TestSender_SendAsBatch_SingleBatch_Success(t *testing.T) {
	g := NewWithT(t)

	azSender := &fakeAzSender{
		NewMessageBatchReturnValue: &azservicebus.MessageBatch{},
		DoSendMessageBatch: func(ctx context.Context, batch *azservicebus.MessageBatch, options *azservicebus.SendMessageBatchOptions) error {
			return nil
		},
	}

	sender := NewSender(azSender, &SenderOptions{
		Marshaller: &DefaultJSONMarshaller{},
	})

	// Create a message (the real batch will fail to add it due to zero size, but we can test the logic)
	msg, err := sender.ToServiceBusMessage(context.Background(), "test")
	g.Expect(err).ToNot(HaveOccurred())

	options := &SendAsBatchOptions{AllowMultipleBatch: true}
	err = sender.SendAsBatch(context.Background(), []*azservicebus.Message{msg}, options)

	// This will error due to real MessageBatch limitations in test, but we test that the logic was exercised
	g.Expect(err).To(HaveOccurred()) // Real MessageBatch fails in tests due to zero max size
	g.Expect(azSender.BatchesCreated).To(Equal(1))
	g.Expect(azSender.BatchesSent).To(Equal(0)) // No batches sent due to AddMessage failure
}

func TestSender_SendAsBatch_MessageTooLarge_SingleMessage(t *testing.T) {
	g := NewWithT(t)

	azSender := &fakeAzSender{
		NewMessageBatchReturnValue: &azservicebus.MessageBatch{}, // Real MessageBatch with 0 max size
	}

	sender := NewSender(azSender, &SenderOptions{
		Marshaller: &DefaultJSONMarshaller{},
	})

	// Create any message - it will be too large for the real MessageBatch with 0 max size
	msg, err := sender.ToServiceBusMessage(context.Background(), "test")
	g.Expect(err).ToNot(HaveOccurred())

	options := &SendAsBatchOptions{AllowMultipleBatch: true}
	err = sender.SendAsBatch(context.Background(), []*azservicebus.Message{msg}, options)

	// Should fail because any message is too large for a real MessageBatch in tests
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("single message is too large"))
	g.Expect(azSender.BatchesCreated).To(Equal(1))
}

func TestSender_SendAsBatch_SingleBatch_TooManyMessages_AllowMultipleFalse(t *testing.T) {
	g := NewWithT(t)
	azSender := &fakeAzSender{
		NewMessageBatchReturnValue: &azservicebus.MessageBatch{},
	}
	sender := NewSender(azSender, &SenderOptions{
		Marshaller: &DefaultJSONMarshaller{},
	})

	// Create multiple messages
	messages := make([]*azservicebus.Message, 3)
	for i := range messages {
		msg, err := sender.ToServiceBusMessage(context.Background(), fmt.Sprintf("test%d", i))
		g.Expect(err).ToNot(HaveOccurred())
		messages[i] = msg
	}

	options := &SendAsBatchOptions{AllowMultipleBatch: false}
	err := sender.SendAsBatch(context.Background(), messages, options)

	// Should fail because messages don't fit in single batch and multiple batches not allowed
	// The real MessageBatch has max size 0 in tests, so AddMessage will fail immediately
	g.Expect(err).To(HaveOccurred())
	g.Expect(azSender.BatchesCreated).To(Equal(1))
}

func TestSender_SendAsBatch_EmptyMessages_Error(t *testing.T) {
	g := NewWithT(t)
	expectedErr := fmt.Errorf("send batch failed")
	azSender := &fakeAzSender{
		NewMessageBatchReturnValue: &azservicebus.MessageBatch{},
		SendMessageBatchErr:        expectedErr,
	}
	sender := NewSender(azSender, nil)

	// Use empty messages which should now return an error directly
	options := &SendAsBatchOptions{AllowMultipleBatch: false}
	err := sender.SendAsBatch(context.Background(), []*azservicebus.Message{}, options)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cannot send empty message array"))
	g.Expect(azSender.BatchesCreated).To(Equal(0)) // No batches created since error returned early
	g.Expect(azSender.SendMessageBatchCalled).To(BeFalse()) // No send attempt made
}

type fakeAzSender struct {
	mu                            sync.RWMutex
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
	CloseErr                      error

	BatchesCreated int // Track how many batches were created
	BatchesSent    int // Track how many batches were sent
}

func (f *fakeAzSender) SendMessage(
	ctx context.Context,
	message *azservicebus.Message,
	options *azservicebus.SendMessageOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SendMessageBatchCalled = true
	f.SendMessageBatchReceivedValue = batch
	f.BatchesSent++

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
	f.mu.Lock()
	defer f.mu.Unlock()
	f.BatchesCreated++

	return f.NewMessageBatchReturnValue, f.NewMessageBatchErr
}

func (f *fakeAzSender) Close(ctx context.Context) error {
	return f.CloseErr
}
