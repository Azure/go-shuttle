package handlers

import (
	"context"

	"github.com/Azure/go-shuttle/tracing/propagation/opencensus"
	"go.opencensus.io/trace"

	servicebus "github.com/Azure/azure-service-bus-go"
)

type TraceType string

const (
	OpenTelemetry TraceType = "OpenTelemetry"
	OpenCensus    TraceType = "OpenCensus"
)

type opencensusTracePropagator struct {
	next      servicebus.Handler
	traceType TraceType
}

func NewTracePropagator(next servicebus.Handler, t TraceType) servicebus.Handler {
	if t == OpenCensus {
		return &opencensusTracePropagator{
			next: next,
		}
	}
	if t == OpenTelemetry {
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
