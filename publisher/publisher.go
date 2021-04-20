package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	common "github.com/Azure/azure-amqp-common-go/v3"
	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/internal/reflection"
	"github.com/devigned/tab"
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

// New creates a new service bus publisher
func New(ctx context.Context, topicName string, opts ...ManagementOption) (*Publisher, error) {
	ctx, s := tab.StartSpan(ctx, "go-shuttle.publisher.New")
	defer s.End()
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

	topicEntity, err := ensureTopic(ctx, topicName, publisher.namespace, publisher.topicManagementOptions...)
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
	ctx, s := tab.StartSpan(ctx, "go-shuttle.publisher.Publish")
	defer s.End()
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
	attempt := 1
	tm := namespace.NewTopicManager()
	ensure := func() (interface{}, error) {
		ctx, span := tab.StartSpan(ctx, "go-shuttle.publisher.ensureTopic")
		span.AddAttributes(tab.Int64Attribute("retry.attempt", int64(attempt)))
		te, err := tm.Get(ctx, name)
		if err == nil {
			return te, nil
		}
		te, err = tm.Put(ctx, name, opts...)
		if err != nil {
			attempt++
			tab.For(ctx).Error(err)
			// let all errors be retryable for now. application only hit this once on topic creation.
			return nil, common.Retryable(err.Error())
		}
		return te, nil
	}
	entity, err := common.Retry(5, 1*time.Second, ensure)
	if err != nil {
		tab.For(ctx).Error(err)
		return nil, err
	}
	return entity.(*servicebus.TopicEntity), nil
}
