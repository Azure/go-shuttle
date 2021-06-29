package handlers

import (
	"context"

	"github.com/Azure/go-shuttle/tracing/propagation"
	"github.com/Azure/go-shuttle/tracing/propagation/opencensus"
	"go.opencensus.io/trace"

	servicebus "github.com/Azure/azure-service-bus-go"
)

type opencensusTracePropagator struct {
	next      servicebus.Handler
	traceType propagation.TraceType
}

// NewTracePropagator creates a handler middleware that will extract the trace context from the incoming messages
// and add a span with the incoming context as remote parent.
// if no trace context is found on the incoming message, a new trace is started.
// TraceType None will return the next handler (making this a no-op)
func NewTracePropagator(t propagation.TraceType, next servicebus.Handler) servicebus.Handler {
	if t == propagation.None {
		return next
	}
	if t == propagation.OpenCensus {
		return &opencensusTracePropagator{
			next: next,
		}
	}
	if t == propagation.OpenTelemetry {
		panic("OpenTelemetry trace propagation is not implemented")
		// not handled for now
		return next
	}
	panic("TracePropagator was not initialized successfully")
}

func (tp *opencensusTracePropagator) Handle(ctx context.Context, msg *servicebus.Message) error {
	spanCtx, ok := opencensus.SpanContextFromMessage(msg)
	if ok {
		var span *trace.Span
		ctx, span = trace.StartSpanWithRemoteParent(ctx, "go-shuttle.listener.spanprogation.Handle", spanCtx)
		defer span.End()
	} else {
		var span *trace.Span
		ctx, span = trace.StartSpan(ctx, "go-shuttle.listener.spanprogation.Handle")
		defer span.End()
	}

	return tp.next.Handle(ctx, msg)
}
