package shuttle

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2/otel"
	"go.opentelemetry.io/otel/trace"
)

// NewTracingHandler is a shuttle middleware that extracts the context from the message Application property if available,
// or from the existing context if not, and starts a span.
func NewTracingHandler(next Handler,
	extractFn func(ctx context.Context, message *azservicebus.ReceivedMessage) (context.Context, trace.Span),
) HandlerFunc {
	if extractFn == nil {
		extractFn = otel.Extract
	}
	return func(ctx context.Context, settler MessageSettler, message *azservicebus.ReceivedMessage) {
		ctx, span := extractFn(ctx, message)
		defer span.End()
		next.Handle(ctx, settler, message)
	}
}

// WithTracePropagation is a sender option to inject the trace context into the message
func WithTracePropagation(ctx context.Context) func(msg *azservicebus.Message) error {
	return func(message *azservicebus.Message) error {
		otel.Inject(ctx, message)
		return nil
	}
}
