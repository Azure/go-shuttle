package opencensus

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/publisher/topic"
	"go.opencensus.io/trace"
)

const (
	TraceIdKey      = "go-shuttle/traceId"
	SpanIdKey       = "go-shuttle/spanId"
	TraceSampledKey = "go-shuttle/traceSampled"
)

func SpanContextFromMessage(msg *servicebus.Message) (trace.SpanContext, bool) {
	sc := trace.SpanContext{}

	// traceID
	traceID, ok := getDecodedBytes(msg.UserProperties, TraceIdKey)
	if !ok {
		return trace.SpanContext{}, false
	}
	copy(sc.TraceID[:], traceID)

	// spanID
	spanID, ok := getDecodedBytes(msg.UserProperties, SpanIdKey)
	if !ok {
		return trace.SpanContext{}, false
	}
	copy(sc.SpanID[:], spanID)

	// startOption
	traceSampled, ok := msg.UserProperties[TraceSampledKey]
	if !ok {
		return trace.SpanContext{}, false
	}
	traceSampledValue, ok := traceSampled.(uint32)
	if !ok {
		return trace.SpanContext{}, false
	}
	sc.TraceOptions = trace.TraceOptions(traceSampledValue)

	// Don't allow all zero traceID or spanID.
	if sc.TraceID == [16]byte{} || sc.SpanID == [8]byte{} {
		return trace.SpanContext{}, false
	}

	return sc, true
}

func SpanContextToMessage(sc trace.SpanContext, msg *servicebus.Message) {
	if msg == nil {
		return
	}
	if msg.UserProperties == nil {
		msg.UserProperties = map[string]interface{}{}
	}
	msg.UserProperties[TraceIdKey] = fmt.Sprintf("%x", sc.TraceID[:])
	msg.UserProperties[SpanIdKey] = fmt.Sprintf("%x", sc.SpanID[:])
	msg.UserProperties[TraceSampledKey] = uint32(sc.TraceOptions)
}

func getDecodedBytes(m map[string]interface{}, key string) ([]byte, bool) {
	iValue, ok := m[key]
	if !ok {
		return nil, false
	}
	hexValue, ok := iValue.(string)
	if !ok {
		return nil, false
	}
	bytesValue, err := hex.DecodeString(hexValue)
	if err != nil {
		return nil, false
	}
	return bytesValue, true
}

func TracePropagation() topic.Option {
	return func(ctx context.Context, msg *servicebus.Message) error {
		if msg == nil {
			return errors.New("message is nil. cannot propagate trace")
		}
		s := trace.FromContext(ctx)
		if s != nil {
			SpanContextToMessage(s.SpanContext(), msg)
		}
		return nil
	}
}
