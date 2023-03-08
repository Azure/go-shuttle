package tracing

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	traceCarrierField = "traceCarrier"
	serviceTracerName = "go-shuttle"
)

var tracer trace.Tracer

func init() {
	tp := tracesdk.NewTracerProvider(tracesdk.WithSampler(tracesdk.AlwaysSample()))
	otel.SetTracerProvider(tp)
	tracer = otel.Tracer(serviceTracerName)
}

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
	var traceCarrier map[string]string

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

		if message.ApplicationProperties != nil {
			traceCarrier = message.ApplicationProperties[traceCarrierField].(map[string]string)
		}
	}

	// extract the remote trace context from the traceCarrier if exists,
	if traceCarrier != nil {
		ctx, _ = getRemoteParentSpan(ctx, traceCarrier)
	}

	ctx, span = tracer.Start(ctx, "receiver.Handle")
	span.SetAttributes(attrs...)
	return ctx, span
}

// This method will extract the remote trace context from the traceCarrier if exists,
// and inject it into the passed in context. If the traceCarrier doesn't contain a
// valid trace context, the passed in context will be returned as is.
// the remote span is returned for UT purpose only
func getRemoteParentSpan(ctx context.Context, traceMap map[string]string) (context.Context, trace.Span) {
	if traceMap != nil {
		propogator := propagation.TraceContext{}
		ctx = propogator.Extract(ctx, propagation.MapCarrier(traceMap))
	}
	return ctx, trace.SpanFromContext(ctx)
}
