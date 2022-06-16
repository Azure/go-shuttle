package publisher

import (
	"context"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-autorest/autorest/adal"

	"github.com/Azure/go-shuttle/common"
	"github.com/Azure/go-shuttle/common/options/publisheropts"
	"github.com/Azure/go-shuttle/marshal"
)

type DeadLetterTarget struct {
}

// ManagementOption provides structure for configuring a new Publisher
type ManagementOption = publisheropts.ManagementOption

// Option provides structure for configuring when starting to publish to a specified queue
type Option = publisheropts.Option

// WithConnectionString configures a publisher with the information provided in a Service Bus connection string
func WithConnectionString(connStr string) ManagementOption {
	return publisheropts.WithConnectionString(connStr)
}

// WithEnvironmentName configures the azure environment used to connect to Servicebus. The environment value used is
// then provided by Azure/go-autorest.
// ref: https://github.com/Azure/go-autorest/blob/c7f947c0610de1bc279f76e6d453353f95cd1bfa/autorest/azure/environments.go#L34
func WithEnvironmentName(environmentName string) ManagementOption {
	return publisheropts.WithEnvironmentName(environmentName)
}

// WithManagedIdentityResourceID configures a publisher with the attached managed identity and the Service bus resource name
func WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) ManagementOption {
	return publisheropts.WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID)
}

// WithManagedIdentityClientID configures a publisher with the attached managed identity and the Service bus resource name
func WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID string) ManagementOption {
	return publisheropts.WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID)
}

func WithToken(serviceBusNamespaceName string, spt *adal.ServicePrincipalToken) ManagementOption {
	return publisheropts.WithToken(serviceBusNamespaceName, spt)
}

// SetDefaultHeader adds a header to every message published using the value specified from the message body
func SetDefaultHeader(headerName, msgKey string) ManagementOption {
	return publisheropts.SetDefaultHeader(headerName, msgKey)
}

// SetDuplicateDetection guarantees that the topic will have exactly-once delivery over a user-defined span of time.
// Defaults to 30 seconds with a maximum of 7 days
func WithDuplicateDetection(window *time.Duration) ManagementOption {
	return func(p common.Publisher) error {
		p.(QueuePublisher).AppendQueueManagementOption(servicebus.QueueEntityWithDuplicateDetection(window))
		return nil
	}
}

// WithForwardDeadLetteredMessagesTo forwards deadlettered messages to a targetable queue, the identity must have management permissions on said queue
func WithForwardDeadLetteredMessagesTo(deadLetterTargetName string, deliveryCount int) ManagementOption {
	return func(p common.Publisher) error {
		qm := p.Namespace().NewQueueManager()

		if _, err := qm.Put(context.Background(), deadLetterTargetName, servicebus.QueueEntityWithMaxDeliveryCount(int32(deliveryCount))); err != nil {
			return err
		}

		deadLetterTarget, err := p.Namespace().NewQueueManager().Get(context.Background(), deadLetterTargetName)
		if err != nil {
			return err
		}
		p.(QueuePublisher).AppendQueueManagementOption(servicebus.QueueEntityWithForwardDeadLetteredMessagesTo(deadLetterTarget))
		return nil
	}
}

// WithDefaultMessageMarshaller sets the Marshaller for the published message. Defaults to Json Marshaller
func WithDefaultMessageMarshaller(marshaller marshal.Marshaller) ManagementOption {
	return publisheropts.SetMessageMarshaller(marshaller)
}

// SetMessageDelay schedules a message in the future
func SetMessageDelay(delay time.Duration) Option {
	return publisheropts.SetMessageDelay(delay)
}

// SetMessageID sets the messageID of the message. Used for duplication detection
func SetMessageID(messageID string) Option {
	return publisheropts.SetMessageID(messageID)
}

// SetCorrelationID sets the SetCorrelationID of the message.
func SetCorrelationID(correlationID string) Option {
	return publisheropts.SetCorrelationID(correlationID)
}
