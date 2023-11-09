package shuttle

import (
	"context"

	"go.opentelemetry.io/otel"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	shuttleotel "github.com/Azure/go-shuttle/v2/otel"
	"go.opentelemetry.io/otel/trace"
)

const (
	serviceTracerName             = "go-shuttle"
	defaultReceiverHandleSpanName = "receiver.Handle"
)

type TracingHandlerOpts struct {
	spanStartOptions []trace.SpanStartOption
	traceProvider    trace.TracerProvider
	spanNameFormat   func(defaultSpanName string, message *azservicebus.ReceivedMessage) string
}

func (t *TracingHandlerOpts) tracer() trace.Tracer {
	if t.traceProvider == nil {
		return otel.Tracer(serviceTracerName)
	}
	return t.traceProvider.Tracer(serviceTracerName)
}

// NewTracingHandler is a shuttle middleware that extracts the context from the message Application property if available,
// or from the existing context if not, and starts a span.
func NewTracingHandler(next Handler, options ...func(t *TracingHandlerOpts)) HandlerFunc {
	t := &TracingHandlerOpts{
		spanNameFormat: func(defaultSpanName string, message *azservicebus.ReceivedMessage) string {
			return defaultSpanName
		},
	}
	for _, opt := range options {
		opt(t)
	}
	return func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
		defaultStartOptions := []trace.SpanStartOption{trace.WithAttributes(shuttleotel.MessageAttributes(message)...)}
		startOptions := append(defaultStartOptions, t.spanStartOptions...)
		ctx, span := t.tracer().Start(
			shuttleotel.Extract(ctx, message),
			t.spanNameFormat(defaultReceiverHandleSpanName, message),
			startOptions...)
		defer span.End()
		next.Handle(ctx, settler, message)
	}
}

func WithTraceProvider(tp trace.TracerProvider) func(t *TracingHandlerOpts) {
	return func(t *TracingHandlerOpts) {
		t.traceProvider = tp
	}
}

func WithReceiverSpanNameFromatter(format func(defaultSpanName string, message *azservicebus.ReceivedMessage) string) func(t *TracingHandlerOpts) {
	return func(t *TracingHandlerOpts) {
		t.spanNameFormat = format
	}
}

func WithSpanStartOptions(options []trace.SpanStartOption) func(t *TracingHandlerOpts) {
	return func(t *TracingHandlerOpts) {
		t.spanStartOptions = options
	}
}

// WithTracePropagation is a sender option to inject the trace context into the message
func WithTracePropagation(ctx context.Context) func(msg *azservicebus.Message) error {
	return func(message *azservicebus.Message) error {
		shuttleotel.Inject(ctx, message)
		return nil
	}
}
