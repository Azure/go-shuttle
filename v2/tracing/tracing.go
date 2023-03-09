package tracing

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	traceCarrierField = "traceCarrier"
	serviceTracerName = "go-shuttle"
)

// NewTracingHandler extracts the context from the message Application property if available, or from the existing
// context if not, and starts a span
func NewTracingHandler(handler shuttle.HandlerFunc) shuttle.HandlerFunc {
	return func(ctx context.Context, settler shuttle.MessageSettler, message *azservicebus.ReceivedMessage) {
		ctx, span := applyTracingMiddleWare(ctx, message)
		defer span.End()
		handler(ctx, settler, message)
	}
}

func applyTracingMiddleWare(ctx context.Context, message *azservicebus.ReceivedMessage) (context.Context, trace.Span) {
	var span trace.Span
	var attrs []attribute.KeyValue

	if message != nil {
		attrs = append(attrs, attribute.String("message.id", message.MessageID))

		if message.CorrelationID != nil {
			attrs = append(attrs, attribute.String("message.correlationId", *message.CorrelationID))
		}

		if message.ScheduledEnqueueTime != nil {
			attrs = append(attrs, attribute.String("message.scheduledEnqueuedTime", message.ScheduledEnqueueTime.String()))
		}

		if message.TimeToLive != nil {
			attrs = append(attrs, attribute.String("message.ttl", message.TimeToLive.String()))
		}

		// extract the remote trace context from the message if exists,
		ctx, _ = getRemoteParentSpan(ctx, message)
	}

	ctx, span = otel.Tracer(serviceTracerName).Start(ctx, "receiver.Handle")
	span.SetAttributes(attrs...)
	return ctx, span
}

// This method will extract the remote trace context from the traceCarrier if exists,
// and inject it into the passed in context. If the traceCarrier doesn't contain a
// valid trace context, the passed in context will be returned as is.
// the remote span is returned for UT purpose only
func getRemoteParentSpan(ctx context.Context, message *azservicebus.ReceivedMessage) (context.Context, trace.Span) {
	propogator := propagation.TraceContext{}
	ctx = propogator.Extract(ctx, carrierAdapterForReceivedMsg(message))
	return ctx, trace.SpanFromContext(ctx)
}

// the implementaion of TextMapCarrier interface for receiver side
type receivedMessageWrapper struct {
	message *azservicebus.ReceivedMessage
}

// wraps a servicebus ReceivedMessage so that it implements the TextMapCarrier interface
func carrierAdapterForReceivedMsg(message *azservicebus.ReceivedMessage) propagation.TextMapCarrier {
	return &receivedMessageWrapper{message: message}
}

func (mw *receivedMessageWrapper) Set(key string, value string) {
	if mw.message.ApplicationProperties == nil {
		mw.message.ApplicationProperties = make(map[string]interface{})
	}
	mw.message.ApplicationProperties[key] = value
}

func (mw *receivedMessageWrapper) Get(key string) string {
	if mw.message.ApplicationProperties == nil || mw.message.ApplicationProperties[key] == nil {
		return ""
	}

	return mw.message.ApplicationProperties[key].(string)
}

func (mw *receivedMessageWrapper) Keys() []string {
	keys := make([]string, 0, len(mw.message.ApplicationProperties))
	for k := range mw.message.ApplicationProperties {
		keys = append(keys, k)
	}
	return keys
}

// this is a sender option to inject the trace context into the message
func PropagateFromContext(ctx context.Context) func(msg *azservicebus.Message) error {
	return func(message *azservicebus.Message) error {
		propogator := propagation.TraceContext{}
		propogator.Inject(ctx, carrierAdapterForMsg(message))
		return nil
	}
}

// the implementaion of TextMapCarrier interface for the sender side
type messageWrapper struct {
	message *azservicebus.Message
}

// wraps a servicebus Message so that it implements the TextMapCarrier interface
func carrierAdapterForMsg(message *azservicebus.Message) propagation.TextMapCarrier {
	return &messageWrapper{message: message}
}

func (mw *messageWrapper) Set(key string, value string) {
	if mw.message.ApplicationProperties == nil {
		mw.message.ApplicationProperties = make(map[string]interface{})
	}
	mw.message.ApplicationProperties[key] = value
}

func (mw *messageWrapper) Get(key string) string {
	if mw.message.ApplicationProperties == nil || mw.message.ApplicationProperties[key] == nil {
		return ""
	}

	return mw.message.ApplicationProperties[key].(string)
}

func (mw *messageWrapper) Keys() []string {
	keys := make([]string, 0, len(mw.message.ApplicationProperties))
	for k := range mw.message.ApplicationProperties {
		keys = append(keys, k)
	}
	return keys
}
