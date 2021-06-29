package opencensus_test

import (
	"context"
	"encoding/hex"
	"testing"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/tracing/propagation/opencensus"
	. "github.com/onsi/gomega"
	"go.opencensus.io/trace"
)

type testArgs struct {
	msgUserProperties map[string]interface{}
	sourceCtx         trace.SpanContext
}

func (t *testArgs) GetMessage() *servicebus.Message {
	return &servicebus.Message{UserProperties: t.msgUserProperties}
}

func TestFromMessage(t *testing.T) {
	tests := []struct {
		name        string
		args        *testArgs
		want        trace.SpanContext
		wantSuccess bool
	}{
		{
			name:        "NoTrace",
			args:        &testArgs{msgUserProperties: map[string]interface{}{}},
			want:        trace.SpanContext{},
			wantSuccess: false,
		},
		{
			name: "EmptyTraceValues",
			args: &testArgs{msgUserProperties: map[string]interface{}{
				opencensus.TraceSampledKey: "",
				opencensus.TraceIdKey:      "",
				opencensus.SpanIdKey:       "",
			}},
			want:        trace.SpanContext{},
			wantSuccess: false,
		},
		{
			name:        "GoodTraceValuesNeverSample",
			args:        getGoodArgs(trace.WithSampler(trace.NeverSample())),
			wantSuccess: true,
		},
		{
			name:        "GoodTraceValuesAlwaysSample",
			args:        getGoodArgs(trace.WithSampler(trace.AlwaysSample())),
			wantSuccess: true,
		},
		{
			name:        "GoodTraceValuesProbabilitySample",
			args:        getGoodArgs(trace.WithSampler(trace.ProbabilitySampler(0.2))),
			wantSuccess: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			msg := tt.args.GetMessage()
			got, gotSuccess := opencensus.SpanContextFromMessage(msg)
			g.Expect(tt.wantSuccess).To(Equal(gotSuccess))
			g.Expect(got).To(Equal(tt.args.sourceCtx))
		})
	}
}

func getGoodArgs(o ...trace.StartOption) *testArgs {
	_, span := trace.StartSpan(context.TODO(), "testSpan", o...)
	traceIdBytes := [16]byte(span.SpanContext().TraceID)
	spanIdBytes := [8]byte(span.SpanContext().SpanID)
	return &testArgs{
		sourceCtx: span.SpanContext(),
		msgUserProperties: map[string]interface{}{
			opencensus.TraceSampledKey: uint32(span.SpanContext().TraceOptions),
			opencensus.TraceIdKey:      hex.EncodeToString(traceIdBytes[:]),
			opencensus.SpanIdKey:       hex.EncodeToString(spanIdBytes[:]),
		}}
}

func TestSpanContextToMessage(t *testing.T) {
	g := NewWithT(t)
	_, span := trace.StartSpan(context.TODO(), "testSpan", trace.WithSampler(trace.AlwaysSample()))
	msg := &servicebus.Message{}
	opencensus.SpanContextToMessage(span.SpanContext(), msg)
	g.Expect(msg.UserProperties[opencensus.TraceIdKey]).ToNot(BeEmpty())
	sc, ok := opencensus.SpanContextFromMessage(msg)
	g.Expect(ok).To(BeTrue())
	g.Expect(sc).To(Equal(span.SpanContext()))
}

func TestTracePropagationOption(t *testing.T) {
	g := NewWithT(t)
	ctx, span := trace.StartSpan(context.TODO(), "testSpan", trace.WithSampler(trace.AlwaysSample()))
	msg := &servicebus.Message{}
	err := opencensus.TracePropagation()(ctx, msg)
	g.Expect(err).ToNot(HaveOccurred())
	sc, ok := opencensus.SpanContextFromMessage(msg)
	g.Expect(ok).To(BeTrue())
	g.Expect(sc).To(Equal(span.SpanContext()))
}
