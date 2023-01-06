package tracing

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2"
	"github.com/devigned/tab"
)

// NewTracingHandler extracts the context from the message Application property if available, or from the existing
// context if not, and starts a span
func NewTracingHandler(handler shuttle.HandlerFunc) shuttle.HandlerFunc {
	return func(ctx context.Context, settler shuttle.MessageSettler, message *azservicebus.ReceivedMessage) {
		ctx, span := tab.StartSpanWithRemoteParent(ctx, "go-shuttle.receiver.Handle", carrierAdapter(message))
		defer span.End()

		if message != nil {
			span.AddAttributes(
				tab.StringAttribute("message.id", message.MessageID),
				tab.StringAttribute("message.correlationId", *message.CorrelationID))
			if message.ScheduledEnqueueTime != nil {
				span.AddAttributes(tab.StringAttribute("message.scheduledEnqueuedTime", message.ScheduledEnqueueTime.String()))
			}
			if message.TimeToLive != nil {
				span.AddAttributes(tab.StringAttribute("message.ttl", message.TimeToLive.String()))
			}
		} else {
			span.Logger().Info("warning: message is nil")
		}
		handler(ctx, settler, message)
	}
}

// carrierAdapter wraps a Received Message so that it implements the tab.Carrier interface
func carrierAdapter(message *azservicebus.ReceivedMessage) tab.Carrier {
	return &messageWrapper{message: message}
}

type messageWrapper struct {
	message *azservicebus.ReceivedMessage
}

// Set implements tab.Carrier interface
func (mw *messageWrapper) Set(key string, value interface{}) {
	if mw.message != nil {
		if mw.message.ApplicationProperties == nil {
			mw.message.ApplicationProperties = make(map[string]interface{})
		}
		mw.message.ApplicationProperties[key] = value
	}
}

// GetKeyValues implements tab.Carrier interface
func (mw *messageWrapper) GetKeyValues() map[string]interface{} {
	if mw.message != nil {
		return mw.message.ApplicationProperties
	}
	return nil
}
