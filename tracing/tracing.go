package tracing

import (
	"context"
	"fmt"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/devigned/tab"
)

func StartSpanFromMessageAndContext(ctx context.Context, operationName string, message *servicebus.Message) (context.Context, tab.Spanner) {
	ctx, span := tab.StartSpan(ctx, operationName)
	if message != nil {
		span.AddAttributes(
			tab.StringAttribute("message.id", message.ID),
			tab.StringAttribute("message.correlationId", message.CorrelationID),
			tab.Int64Attribute("message.deliveryCount", int64(message.DeliveryCount)))
		if message.SystemProperties != nil {
			if message.SystemProperties.SequenceNumber != nil {
				span.AddAttributes(tab.Int64Attribute("message.sequenceNumber", *message.SystemProperties.SequenceNumber))
			}
			if message.SystemProperties.EnqueuedTime != nil {
				span.AddAttributes(tab.StringAttribute("message.sequenceNumber", message.SystemProperties.EnqueuedTime.String()))
			}
			if message.SystemProperties.LockedUntil != nil {
				span.AddAttributes(tab.StringAttribute("message.lockedUntil", message.SystemProperties.EnqueuedTime.String()))
			}
			if message.TTL != nil {
				span.AddAttributes(tab.StringAttribute("message.TTL", message.TTL.String()))
			}
		}
	} else {
		span.Logger().Error(fmt.Errorf("message is nil"))
	}
	return ctx, span
}
