package message

import (
	"context"
	"fmt"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/devigned/tab"
)

func startSpanFromMessageAndContext(ctx context.Context, operationName string, message *servicebus.Message) (context.Context, tab.Spanner) {
	ctx, span := tab.StartSpan(ctx, operationName)
	if message != nil {
		attrs := []tab.Attribute{tab.StringAttribute("id", message.ID),
			tab.Int64Attribute("deliveryCount", int64(message.DeliveryCount))}

		if message.SystemProperties != nil && message.SystemProperties.SequenceNumber != nil {
			attrs = append(attrs, tab.Int64Attribute("sequenceNumber", *message.SystemProperties.SequenceNumber))
		}

		span.AddAttributes(attrs...)
	} else {
		span.Logger().Error(fmt.Errorf("message is nil"))
	}

	return ctx, span
}
