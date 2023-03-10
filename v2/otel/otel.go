package otel

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	serviceTracerName = "go-shuttle"
)

func Extract(ctx context.Context, message *azservicebus.ReceivedMessage) (context.Context, trace.Span) {
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
	ctx = propogator.Extract(ctx, ReceivedMessageCarrierAdapter(message))
	return ctx, trace.SpanFromContext(ctx)
}

// the implementaion of TextMapCarrier interface for receiver side
type receivedMessageWrapper struct {
	message *azservicebus.ReceivedMessage
}

// ReceivedMessageCarrierAdapter wraps a servicebus ReceivedMessage so that it implements the TextMapCarrier interface
func ReceivedMessageCarrierAdapter(message *azservicebus.ReceivedMessage) propagation.TextMapCarrier {
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

func Inject(ctx context.Context, msg *azservicebus.Message) {
	propogator := propagation.TraceContext{}
	propogator.Inject(ctx, MessageCarrierAdapter(msg))
}

// the implementaion of TextMapCarrier interface for the sender side
type messageWrapper struct {
	message *azservicebus.Message
}

// MessageCarrierAdapter wraps a azservicebus.Message so that it implements the propagation.TextMapCarrier interface
func MessageCarrierAdapter(message *azservicebus.Message) propagation.TextMapCarrier {
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
