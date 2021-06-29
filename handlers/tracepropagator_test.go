package handlers_test

import (
	"context"
	"testing"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/handlers"
	"github.com/Azure/go-shuttle/tracing/propagation/opencensus"
	. "github.com/onsi/gomega"
	"go.opencensus.io/trace"
)

type RecorderHandler struct {
	inCtx context.Context
	msg   *servicebus.Message
}

func (h *RecorderHandler) Handle(ctx context.Context, msg *servicebus.Message) error {
	h.inCtx = ctx
	msg = msg
	return nil
}

func TestWhenMessageHasSpan(t *testing.T) {
	g := NewWithT(t)
	record := &RecorderHandler{}
	msg := &servicebus.Message{}
	_, span := trace.StartSpan(context.TODO(), "testSpan", trace.WithSampler(trace.AlwaysSample()))
	opencensus.SpanContextToMessage(span.SpanContext(), msg)
	h := handlers.NewTracePropagator(record, handlers.OpenCensus)
	err := h.Handle(context.TODO(), msg)
	g.Expect(err).ToNot(HaveOccurred())
	resSpan := trace.FromContext(record.inCtx)
	g.Expect(resSpan.SpanContext().TraceID).To(Equal(span.SpanContext().TraceID))
}

func TestWhenMessageDoesNotHaveSpan(t *testing.T) {
	g := NewWithT(t)
	record := &RecorderHandler{}
	msg := &servicebus.Message{}
	_, span := trace.StartSpan(context.TODO(), "testSpan", trace.WithSampler(trace.AlwaysSample()))
	h := handlers.NewTracePropagator(record, handlers.OpenCensus)
	err := h.Handle(context.TODO(), msg)
	g.Expect(err).ToNot(HaveOccurred())
	resSpan := trace.FromContext(record.inCtx)
	g.Expect(resSpan.SpanContext().TraceID).ToNot(Equal(span.SpanContext().TraceID))
}
