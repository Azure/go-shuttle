package publisher

import (
	"errors"
	"time"

	"github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-shuttle/internal/aad"
	servicebus2 "github.com/Azure/go-shuttle/internal/servicebus"
)

// ManagementOption provides structure for configuring a new Publisher
type ManagementOption func(p *Publisher) error

// Option provides structure for configuring when starting to publish to a specified topic
type Option func(msg *servicebus.Message) error

// WithConnectionString configures a publisher with the information provided in a Service Bus connection string
func WithConnectionString(connStr string) ManagementOption {
	return func(p *Publisher) error {
		if connStr == "" {
			return errors.New("no Service Bus connection string provided")
		}
		ns, err := servicebus.NewNamespace(servicebus.NamespaceWithConnectionString(connStr))
		if err != nil {
			return err
		}
		p.namespace = ns
		return nil
	}
}

// WithEnvironmentName configures the azure environment used to connect to Servicebus. The environment value used is
// then provided by Azure/go-autorest.
// ref: https://github.com/Azure/go-autorest/blob/c7f947c0610de1bc279f76e6d453353f95cd1bfa/autorest/azure/environments.go#L34
func WithEnvironmentName(environmentName string) ManagementOption {
	return func(p *Publisher) error {
		if environmentName == "" {
			return errors.New("cannot use empty environment name")
		}
		var env azure.Environment
		var err error
		if env, err = azure.EnvironmentFromName(environmentName); err != nil {
			return err
		}
		p.namespace.Environment = env
		p.namespace.Suffix = env.ServiceBusEndpointSuffix
		return nil
	}
}

// WithManagedIdentityResourceID configures a publisher with the attached managed identity and the Service bus resource name
func WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) ManagementOption {
	return func(p *Publisher) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		ns, err := servicebus.NewNamespace(servicebus2.NamespaceWithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID))
		if err != nil {
			return err
		}
		p.namespace = ns
		return nil
	}
}

// WithManagedIdentityClientID configures a publisher with the attached managed identity and the Service bus resource name
func WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID string) ManagementOption {
	return func(p *Publisher) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		ns, err := servicebus.NewNamespace(servicebus2.NamespaceWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID))
		if err != nil {
			return err
		}
		p.namespace = ns
		return nil
	}
}

func WithToken(serviceBusNamespaceName string, spt *adal.ServicePrincipalToken) ManagementOption {
	return func(p *Publisher) error {
		if spt == nil {
			return errors.New("cannot provide a nil token")
		}
		ns, err := servicebus.NewNamespace(servicebus2.NamespaceWithTokenProvider(serviceBusNamespaceName, aad.AsJWTTokenProvider(spt)))
		if err != nil {
			return err
		}
		p.namespace = ns
		return nil
	}
}

// SetDefaultHeader adds a header to every message published using the value specified from the message body
func SetDefaultHeader(headerName, msgKey string) ManagementOption {
	return func(p *Publisher) error {
		if p.headers == nil {
			p.headers = make(map[string]string)
		}
		p.headers[headerName] = msgKey
		return nil
	}
}

// SetDuplicateDetection guarantees that the topic will have exactly-once delivery over a user-defined span of time.
// Defaults to 30 seconds with a maximum of 7 days
func WithDuplicateDetection(window *time.Duration) ManagementOption {
	return func(p *Publisher) error {
		p.topicManagementOptions = append(p.topicManagementOptions, servicebus.TopicWithDuplicateDetection(window))
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
