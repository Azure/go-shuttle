package handlers

import (
	"context"

	"github.com/Azure/go-shuttle/tracing/propagation"
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
	spanCtx, ok := propagation.SpanContextFromMessage(msg)
	if !ok {
		ctx, _ = trace.StartSpanWithRemoteParent(ctx, "go-shuttle.listener.spanprogation.Handle", spanCtx)
	}
	return tp.next.Handle(ctx, msg)
}
