package shuttle

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"go.opentelemetry.io/otel/propagation"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
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

func TestHandlers_SetMessageTTL(t *testing.T) {
	blankMsg := &azservicebus.Message{}
	ttl := time.Duration(10 * time.Second)
	handler := SetMessageTTL(ttl)
	if err := handler(blankMsg); err != nil {
		t.Errorf("Unexpected error in set message TTL at test: %s", err)
	}
	if *blankMsg.TimeToLive != ttl {
		t.Errorf("for message TTL at expected %s, got %s", ttl, *blankMsg.TimeToLive)
	}
}

func TestHandlers_SetTraceCarrier(t *testing.T) {
	blankMsg := &azservicebus.Message{}
	tp := tracesdk.NewTracerProvider(tracesdk.WithSampler(tracesdk.AlwaysSample()))

	// fake a remote span
	remoteCtx, remoteSpan := tp.Tracer("test-tracer").Start(context.Background(), "remote-span")
	remoteSpan.End()

	// inject the remote span into a trace carrier
	traceCarrier := make(map[string]string)
	propogator := propagation.TraceContext{}
	propogator.Inject(remoteCtx, propagation.MapCarrier(traceCarrier))

	// set the trace carrier on the message
	handler := SetTraceCarrier(traceCarrier)
	if err := handler(blankMsg); err != nil {
		t.Errorf("Unexpected error in set trace carrier test: %s", err)
	}

	if blankMsg.ApplicationProperties == nil {
		t.Errorf("for trace carrier expected application properties to be set")
	}

	if blankMsg.ApplicationProperties[traceCarrierField] == nil {
		t.Errorf("for trace carrier expected %s field to be set", traceCarrierField)
	}

	// extract the span from trace carrier from the message
	traceCarrier = blankMsg.ApplicationProperties[traceCarrierField].(map[string]string)
	ctx := propogator.Extract(context.TODO(), propagation.MapCarrier(traceCarrier))
	extractedSpan := trace.SpanFromContext(ctx)

	if !extractedSpan.SpanContext().IsValid() || !extractedSpan.SpanContext().IsRemote() {
		t.Errorf("expected extracted span to be valid and remote")
	}

	if reflect.DeepEqual(extractedSpan, remoteSpan) {
		t.Errorf("expected extracted span to be the same with remote span")
	}
}
