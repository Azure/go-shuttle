package publisheropts

import (
	"errors"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-autorest/autorest/adal"

	"github.com/Azure/go-shuttle/common"
	"github.com/Azure/go-shuttle/internal/aad"
	sbinternal "github.com/Azure/go-shuttle/internal/servicebus"
	"github.com/Azure/go-shuttle/marshal"
)

// ManagementOption provides structure for configuring a new Publisher
type ManagementOption func(p common.Publisher) error

// Option provides structure for configuring when starting to publish to a specified queue
type Option func(msg *servicebus.Message) error

// WithConnectionString configures a publisher with the information provided in a Service Bus connection string
func WithConnectionString(connStr string) ManagementOption {
	return func(p common.Publisher) error {
		if connStr == "" {
			return errors.New("no Service Bus connection string provided")
		}
		return servicebus.NamespaceWithConnectionString(connStr)(p.Namespace())
	}
}

// WithEnvironmentName configures the azure environment used to connect to Servicebus. The environment value used is
// then provided by Azure/go-autorest.
// ref: https://github.com/Azure/go-autorest/blob/c7f947c0610de1bc279f76e6d453353f95cd1bfa/autorest/azure/environments.go#L34
func WithEnvironmentName(environmentName string) ManagementOption {
	return func(p common.Publisher) error {
		if environmentName == "" {
			return errors.New("cannot use empty environment name")
		}
		return servicebus.NamespaceWithAzureEnvironment(p.Namespace().Name, environmentName)(p.Namespace())
	}
}

// WithManagedIdentityResourceID configures a publisher with the attached managed identity and the Service bus resource name
func WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) ManagementOption {
	return func(p common.Publisher) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		return sbinternal.NamespaceWithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID)(p.Namespace())
	}
}

// WithManagedIdentityClientID configures a publisher with the attached managed identity and the Service bus resource name
func WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID string) ManagementOption {
	return func(p common.Publisher) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		return sbinternal.NamespaceWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID)(p.Namespace())
	}
}

func WithToken(serviceBusNamespaceName string, spt *adal.ServicePrincipalToken) ManagementOption {
	return func(p common.Publisher) error {
		if spt == nil {
			return errors.New("cannot provide a nil token")
		}
		return sbinternal.NamespaceWithTokenProvider(serviceBusNamespaceName, aad.AsJWTTokenProvider(spt))(p.Namespace())
	}
}

// SetDefaultHeader adds a header to every message published using the value specified from the message body
func SetDefaultHeader(headerName, msgKey string) ManagementOption {
	return func(p common.Publisher) error {
		p.AppendHeader(headerName, msgKey)
		return nil
	}
}

// SetDefaultHeader adds a header to every message published using the value specified from the message body
func SetMessageMarshaller(marshaller marshal.Marshaller) ManagementOption {
	return func(p common.Publisher) error {
		p.SetMarshaller(marshaller)
		return nil
	}
}

// SetMessageDelay schedules a message in the future
func SetMessageDelay(delay time.Duration) Option {
	return func(msg *servicebus.Message) error {
		if msg == nil {
			return errors.New("message is nil. cannot assign message delay")
		}
		msg.ScheduleAt(time.Now().Add(delay))
		return nil
	}
}

// SetMessageID sets the messageID of the message. Used for duplication detection
func SetMessageID(messageID string) Option {
	return func(msg *servicebus.Message) error {
		if msg == nil {
			return errors.New("message is nil. cannot assign message ID")
		}
		msg.ID = messageID
		return nil
	}
}

// SetCorrelationID sets the SetCorrelationID of the message.
func SetCorrelationID(correlationID string) Option {
	return func(msg *servicebus.Message) error {
		if msg == nil {
			return errors.New("message is nil. cannot assign correlation ID")
		}
		msg.CorrelationID = correlationID
		return nil
	}
}

// SetUserProperty sets a SetUserProperty on the message.
func SetUserProperty(key string, value interface{}) Option {
	return func(msg *servicebus.Message) error {
		if msg == nil {
			return errors.New("message is nil. cannot set user property")
		}
		if msg.UserProperties == nil {
			msg.UserProperties = map[string]interface{}{}
		}
		msg.UserProperties[key] = value
		return nil
	}
}

// SetMessage provides a func that will configure the message before sending it.
func SetMessage(messageConfigurator func(msg *servicebus.Message) error) Option {
	return messageConfigurator
}
