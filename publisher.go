package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/keikumata/azure-pub-sub/internal/reflection"
)

// Publisher is a struct to contain service bus entities relevant to publishing to a topic
type Publisher struct {
	namespace   *servicebus.Namespace
	topicSender *servicebus.Sender
	headers     map[string]string
	sbMsg       *servicebus.Message
}

// PublisherManagementOption provides structure for configuring a new Publisher
type PublisherManagementOption func(p *Publisher) error

// PublisherOption provides structure for configuring when starting to publish to a specified topic
type PublisherOption func(p *Publisher) error

// PublisherWithConnectionString configures a publisher with the information provided in a Service Bus connection string
func PublisherWithConnectionString(connStr string) PublisherManagementOption {
	return func(p *Publisher) error {
		ns, err := getNamespace(connStr)
		if err != nil {
			return err
		}
		p.namespace = ns
		return nil
	}
}

// SetDefaultHeader adds a header to every message published using the value specified from the message body
func SetDefaultHeader(headerName, msgKey string) PublisherManagementOption {
	return func(p *Publisher) error {
		if p.headers == nil {
			p.headers = make(map[string]string)
		}
		p.headers[headerName] = msgKey
		return nil
	}
}

// SetMessageDelay schedules a message in the future
func SetMessageDelay(delay time.Duration) PublisherOption {
	return func(p *Publisher) error {
		if p.sbMsg == nil {
			return errors.New("Cannot assign message delay")
		}
		p.sbMsg.ScheduleAt(time.Now().Add(delay))
		return nil
	}
}

// NewPublisher creates a new service bus publisher
func NewPublisher(topicName string, opts ...PublisherManagementOption) (*Publisher, error) {
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
	topicEntity, err := ensureTopic(context.Background(), topicName, publisher.namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic: %w", err)
	}
	topic, err := publisher.namespace.NewTopic(topicEntity.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create new topic %s: %w", topicEntity.Name, err)
	}
	defer func() {
		_ = topic.Close(context.Background())
	}()

	topicSender, err := topic.NewSender(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create new topic sender for topic %s: %w", topicEntity.Name, err)
	}
	publisher.topicSender = topicSender

	return publisher, nil
}

// Publish publishes to the pre-configured Service Bus topic
func (p *Publisher) Publish(ctx context.Context, msg interface{}, opts ...PublisherOption) error {
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
	p.sbMsg = sbMsg

	// now apply publishing options
	for _, opt := range opts {
		err := opt(p)
		if err != nil {
			return err
		}
	}

	// finally, send
	err = p.topicSender.Send(ctx, sbMsg)
	if err != nil {
		return fmt.Errorf("failed to send message to topic %s: %w", p.topicSender.Name, err)
	}
	return nil
}

func ensureTopic(ctx context.Context, name string, namespace *servicebus.Namespace) (*servicebus.TopicEntity, error) {
	tm := namespace.NewTopicManager()
	te, err := tm.Get(ctx, name)
	if err == nil {
		return te, nil
	}

	return tm.Put(ctx, name)
}
