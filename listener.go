package pubsub

import (
	"context"
	"errors"
	"fmt"

	servicebus "github.com/Azure/azure-service-bus-go"
)

const (
	defaultSubscriptionName = "default"
)

// Handle is a func to handle the message received from a subscription
type Handle func(ctx context.Context, message string) error

// Listener is a struct to contain service bus entities relevant to subscribing to a publisher topic
type Listener struct {
	namespace          *servicebus.Namespace
	topicEntity        *servicebus.TopicEntity
	subscriptionEntity *servicebus.SubscriptionEntity
	listenerHandle     *servicebus.ListenerHandle
}

// ListenerManagementOption provides structure for configuring a new Listener
type ListenerManagementOption func(l *Listener) error

// ListenerOption provides structure for configuring when starting to listen to a specified topic
type ListenerOption func(l *Listener) error

// ListenerWithConnectionString configures a listener with the information provided in a Service Bus connection string
func ListenerWithConnectionString(connStr string) ListenerManagementOption {
	return func(l *Listener) error {
		ns, err := getNamespace(connStr)
		if err != nil {
			return err
		}
		l.namespace = ns
		return nil
	}
}

// SetSubscriptionName configures the subscription name of the subscription to listen to
func SetSubscriptionName(name string) ListenerOption {
	return func(l *Listener) error {
		ctx := context.Background()
		subscriptionEntity, err := getSubscriptionEntity(ctx, name, l.namespace, l.topicEntity)
		if err != nil {
			return fmt.Errorf("failed to get subscription: %w", err)
		}
		l.subscriptionEntity = subscriptionEntity
		return nil
	}
}

// SetSubscriptionFilter configures a filter of the subscription to listen to
func SetSubscriptionFilter(filterName string, filter servicebus.FilterDescriber) ListenerOption {
	return func(l *Listener) error {
		if len(filterName) == 0 || filter == nil {
			return errors.New("filter name or filter cannot be zero value")
		}
		ctx := context.Background()
		sm, err := l.namespace.NewSubscriptionManager(l.topicEntity.Name)
		if err != nil {
			return err
		}
		return ensureFilterRule(ctx, sm, l.subscriptionEntity.Name, filterName, filter)
	}
}

// NewListener creates a new service bus listener
func NewListener(opts ...ListenerManagementOption) (*Listener, error) {
	ns, err := servicebus.NewNamespace()
	if err != nil {
		return nil, err
	}
	listener := &Listener{namespace: ns}
	for _, opt := range opts {
		err := opt(listener)
		if err != nil {
			return nil, err
		}
	}

	return listener, nil
}

// Listen waits for a message from the Service Bus Topic subscription
func (l *Listener) Listen(ctx context.Context, handle Handle, topicName string, opts ...ListenerOption) error {
	// default setup
	topicEntity, err := getTopicEntity(ctx, topicName, l.namespace)
	if err != nil {
		return fmt.Errorf("failed to get topic: %w", err)
	}
	l.topicEntity = topicEntity
	subscriptionEntity, err := getSubscriptionEntity(ctx, defaultSubscriptionName, l.namespace, topicEntity)
	if err != nil {
		return fmt.Errorf("failed to get subscriptionEntity: %w", err)
	}
	l.subscriptionEntity = subscriptionEntity

	// apply listener options
	for _, opt := range opts {
		err := opt(l)
		if err != nil {
			return err
		}
	}

	// Generate new topic client
	topic, err := l.namespace.NewTopic(l.topicEntity.Name)
	if err != nil {
		return fmt.Errorf("failed to create new topic %s: %w", l.topicEntity.Name, err)
	}
	defer func() {
		_ = topic.Close(ctx)
	}()

	// Generate new subscription client
	sub, err := topic.NewSubscription(l.subscriptionEntity.Name)
	if err != nil {
		return fmt.Errorf("failed to create new subscription %s: %w", l.subscriptionEntity.Name, err)
	}
	subReceiver, err := sub.NewReceiver(ctx)
	if err != nil {
		return fmt.Errorf("failed to create new subscription receiver %s: %w", subReceiver.Name, err)
	}

	// Create a handle class that has that function
	listenerHandle := subReceiver.Listen(ctx, servicebus.HandlerFunc(
		func(ctx context.Context, message *servicebus.Message) error {
			err := handle(ctx, string(message.Data))
			if err != nil {
				err = message.Abandon(ctx)
				return err
			}
			return message.Complete(ctx)
		},
	))
	l.listenerHandle = listenerHandle
	<-listenerHandle.Done()

	if err := subReceiver.Close(ctx); err != nil {
		return fmt.Errorf("error shutting down service bus subscription. %w", err)
	}
	return listenerHandle.Err()
}

// Close closes the listener if an active listener exists
func (l *Listener) Close(ctx context.Context) error {
	if l.listenerHandle == nil {
		return errors.New("no active listener. cannot close")
	}
	if err := l.listenerHandle.Close(ctx); err != nil {
		l.listenerHandle = nil
		return fmt.Errorf("error shutting down service bus subscription. %w", err)
	}
	return nil
}

func getNamespace(connStr string) (*servicebus.Namespace, error) {
	if connStr == "" {
		return nil, errors.New("no Service Bus connection string provided")
	}
	// Create a client to communicate with a Service Bus Namespace.
	namespace, err := servicebus.NewNamespace(servicebus.NamespaceWithConnectionString(connStr))
	if err != nil {
		return nil, err
	}
	return namespace, nil
}

func getTopicEntity(ctx context.Context, topicName string, namespace *servicebus.Namespace) (*servicebus.TopicEntity, error) {
	tm := namespace.NewTopicManager()
	return tm.Get(ctx, topicName)
}

func getSubscriptionEntity(
	ctx context.Context,
	subscriptionName string,
	ns *servicebus.Namespace,
	te *servicebus.TopicEntity) (*servicebus.SubscriptionEntity, error) {
	subscriptionManager, err := ns.NewSubscriptionManager(te.Name)
	if err != nil {
		return nil, err
	}

	subEntity, err := ensureSubscription(ctx, subscriptionManager, subscriptionName)
	if err != nil {
		return nil, err
	}

	return subEntity, nil
}

func ensureSubscription(ctx context.Context, sm *servicebus.SubscriptionManager, name string) (*servicebus.SubscriptionEntity, error) {
	subEntity, err := sm.Get(ctx, name)
	if err == nil {
		return subEntity, nil
	}

	return sm.Put(ctx, name)
}

func ensureFilterRule(
	ctx context.Context,
	sm *servicebus.SubscriptionManager,
	subName string,
	filterName string,
	filter servicebus.FilterDescriber) error {
	rules, err := sm.ListRules(ctx, subName)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.Name == filterName {
			if compareFilter(rule.Filter, filter.ToFilterDescription()) {
				// exit early cause the rule with the same filter already exists
				return nil
			}
			// update existing rule with new filter
			err = sm.DeleteRule(ctx, subName, rule.Name)
			if err != nil {
				return err
			}
			_, err = sm.PutRule(ctx, subName, filterName, filter)
			return err
		}
	}

	// remove the default rule, which is the "TrueFilter" that accepts all messages
	err = sm.DeleteRule(ctx, subName, "$Default")
	if err != nil {
		return err
	}
	_, err = sm.PutRule(ctx, subName, filterName, filter)
	return err
}

func compareFilter(f1, f2 servicebus.FilterDescription) bool {
	if f1.Type == "CorrelationFilter" {
		return f1.CorrelationFilter.CorrelationID == f2.CorrelationFilter.CorrelationID &&
			f1.CorrelationFilter.Label == f2.CorrelationFilter.Label
	}
	// other than CorrelationFilter all other filters use a SQLExpression
	return *f1.SQLExpression == *f2.SQLExpression
}
