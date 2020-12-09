package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-shuttle/internal/aad"
	"github.com/Azure/go-shuttle/internal/reflection"
	servicebusinternal "github.com/Azure/go-shuttle/internal/servicebus"
)

// Publisher is a struct to contain service bus entities relevant to publishing to a topic
type Publisher struct {
	namespace              *servicebus.Namespace
	topic                  *servicebus.Topic
	headers                map[string]string
	topicManagementOptions []servicebus.TopicManagementOption
}

func (p *Publisher) Namespace() *servicebus.Namespace {
	return p.namespace
}

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

// WithManagedIdentityResourceID configures a publisher with the attached managed identity and the Service bus resource name
func WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) ManagementOption {
	return func(p *Publisher) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		ns, err := servicebus.NewNamespace(servicebusinternal.NamespaceWithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID))
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
		ns, err := servicebus.NewNamespace(servicebusinternal.NamespaceWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID))
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
		ns, err := servicebus.NewNamespace(servicebusinternal.NamespaceWithTokenProvider(serviceBusNamespaceName, aad.AsJWTTokenProvider(spt)))
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

// New creates a new service bus publisher
func New(topicName string, opts ...ManagementOption) (*Publisher, error) {
	ns, err := servicebus.NewNamespace()
	if err != nil {
		return nil, err
	}
	publisher := &Publisher{namespace: ns}
	for _, opt := range opts {
		err := opt(publisher)
		if err != nil {
			return nil, err
		}
	}
	topicEntity, err := ensureTopic(context.Background(), topicName, publisher.namespace, publisher.topicManagementOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic: %w", err)
	}
	topic, err := publisher.namespace.NewTopic(topicEntity.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create new topic %s: %w", topicEntity.Name, err)
	}

	publisher.topic = topic
	return publisher, nil
}

// Publish publishes to the pre-configured Service Bus topic
func (p *Publisher) Publish(ctx context.Context, msg interface{}, opts ...Option) error {
	msgJSON, err := json.Marshal(msg)

	// adding in user properties to enable filtering on listener side
	sbMsg := servicebus.NewMessageFromString(string(msgJSON))
	sbMsg.UserProperties = make(map[string]interface{})
	sbMsg.UserProperties["type"] = reflection.GetType(msg)

	// add in custom headers setup at initialization time
	for headerName, headerKey := range p.headers {
		val := reflection.GetReflectionValue(msg, headerKey)
		if val != nil {
			sbMsg.UserProperties[headerName] = val
		}
	}

	// now apply publishing options
	for _, opt := range opts {
		err := opt(sbMsg)
		if err != nil {
			return err
		}
	}

	// finally, send
	err = p.topic.Send(ctx, sbMsg)
	if err != nil {
		return fmt.Errorf("failed to send message to topic %s: %w", p.topic.Name, err)
	}
	return nil
}

func ensureTopic(ctx context.Context, name string, namespace *servicebus.Namespace, opts ...servicebus.TopicManagementOption) (*servicebus.TopicEntity, error) {
	tm := namespace.NewTopicManager()
	te, err := tm.Get(ctx, name)
	if err == nil {
		return te, nil
	}

	return tm.Put(ctx, name, opts...)
}

// Delete the given topic
func DeleteTopic(topicName string, opts ...ManagementOption) error {
	ns, err := servicebus.NewNamespace()
	if err != nil {
		return err
	}
	publisher := &Publisher{namespace: ns}
	for _, opt := range opts {
		err := opt(publisher)
		if err != nil {
			return err
		}
	}

	tm := publisher.namespace.NewTopicManager()

	return tm.Delete(context.Background(), topicName)
}
