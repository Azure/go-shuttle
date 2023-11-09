package shuttle

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2/otel"
	"go.opentelemetry.io/otel/trace"
)

type TracingHandlerOpts struct {
	enrichSpan func(context.Context, trace.Span)
}

// NewTracingHandler is a shuttle middleware that extracts the context from the message Application property if available,
// or from the existing context if not, and starts a span.
func NewTracingHandler(next Handler, options ...func(t *TracingHandlerOpts)) HandlerFunc {
	t := &TracingHandlerOpts{}
	for _, opt := range options {
		opt(t)
	}
	return func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
		ctx, span := otel.Extract(ctx, message)
		defer span.End()
		if t.enrichSpan != nil {
			t.enrichSpan(ctx, span)
		}
		next.Handle(ctx, settler, message)
	}
}

func WithSpanEnrichment(enrichSpan func(context.Context, trace.Span)) func(t *TracingHandlerOpts) {
	return func(t *TracingHandlerOpts) {
		t.enrichSpan = enrichSpan
	}
}

// WithTracePropagation is a sender option to inject the trace context into the message
func WithTracePropagation(ctx context.Context) func(msg *azservicebus.Message) error {
	return func(message *azservicebus.Message) error {
		otel.Inject(ctx, message)
		return nil
	}
}
