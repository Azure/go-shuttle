package pubsub

import (
	"context"
	"errors"
	"fmt"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/keikumata/azure-pub-sub/message"

	servicebusinternal "github.com/keikumata/azure-pub-sub/internal/servicebus"
)

const (
	defaultSubscriptionName = "default"
)

// Listener is a struct to contain service bus entities relevant to subscribing to a publisher topic
type Listener struct {
	namespace          *servicebus.Namespace
	topicEntity        *servicebus.TopicEntity
	subscriptionEntity *servicebus.SubscriptionEntity
	listenerHandle     *servicebus.ListenerHandle
	topicName          string
	subscriptionName   string
	filterDefinitions  []*filterDefinition
}

// ListenerManagementOption provides structure for configuring a new Listener
type ListenerManagementOption func(l *Listener) error

// ListenerOption provides structure for configuring when starting to listen to a specified topic
type ListenerOption func(l *Listener) error

// ListenerWithConnectionString configures a listener with the information provided in a Service Bus connection string
func ListenerWithConnectionString(connStr string) ListenerManagementOption {
	return func(l *Listener) error {
		if connStr == "" {
			return errors.New("no Service Bus connection string provided")
		}
		ns, err := getNamespace(servicebus.NamespaceWithConnectionString(connStr))
		if err != nil {
			return err
		}
		l.namespace = ns
		return nil
	}
}

// Deprecated use ListenerWithManagedIdentityClientID instead
func ListenerWithManagedIdentity(serviceBusNamespaceName, managedIdentityClientID string) ListenerManagementOption {
	return ListenerWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID)
}

// ListenerWithManagedIdentityClientID configures a listener with the attached managed identity and the Service bus resource name
func ListenerWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID string) ListenerManagementOption {
	return func(l *Listener) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		ns, err := getNamespace(servicebusinternal.NamespaceWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID))
		if err != nil {
			return err
		}
		l.namespace = ns
		return nil
	}
}

// ListenerWithManagedIdentityResourceID configures a listener with the attached managed identity and the Service bus resource name
func ListenerWithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) ListenerManagementOption {
	return func(l *Listener) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		ns, err := getNamespace(servicebusinternal.NamespaceWithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID))
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
		l.subscriptionName = name
		return nil
	}
}

type filterDefinition struct {
	Name   string
	Filter servicebus.FilterDescriber
}

// SetSubscriptionFilter configures a filter of the subscription to listen to
func SetSubscriptionFilter(filterName string, filter servicebus.FilterDescriber) ListenerOption {
	return func(l *Listener) error {
		if len(filterName) == 0 || filter == nil {
			return errors.New("filter name or filter cannot be zero value")
		}
		l.filterDefinitions = append(l.filterDefinitions, &filterDefinition{filterName, filter})
		return nil
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

func setTopicEntity(ctx context.Context, l *Listener) error {
	if l.topicEntity != nil {
		return nil
	}
	topicEntity, err := getTopicEntity(ctx, l.topicName, l.namespace)
	if err != nil {
		return fmt.Errorf("failed to get topic: %w", err)
	}
	l.topicEntity = topicEntity
	return nil
}

func setSubscriptionEntity(ctx context.Context, l *Listener) error {
	if l.subscriptionEntity != nil {
		return nil
	}
	if l.subscriptionName == "" {
		l.subscriptionName = defaultSubscriptionName
	}
	subscriptionEntity, err := getSubscriptionEntity(ctx, l.subscriptionName, l.namespace, l.topicEntity)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}
	l.subscriptionEntity = subscriptionEntity
	return nil
}

func setSubscriptionFilters(ctx context.Context, l *Listener) error {
	if len(l.filterDefinitions) == 0 {
		return nil
	}
	sm, err := l.namespace.NewSubscriptionManager(l.topicEntity.Name)
	if err != nil {
		return fmt.Errorf("could no create subscription manager: %w", err)
	}
	for _, d := range l.filterDefinitions {
		if err := ensureFilterRule(ctx, sm, l.subscriptionName, d.Name, d.Filter); err != nil {
			return err
		}
	}
	return nil
}

// Listen waits for a message from the Service Bus Topic subscription
func (l *Listener) Listen(ctx context.Context, handler message.Handler, topicName string, opts ...ListenerOption) error {
	l.topicName = topicName
	// apply listener options
	for _, opt := range opts {
		err := opt(l)
		if err != nil {
			return err
		}
	}
	if err := setTopicEntity(ctx, l); err != nil {
		return err
	}
	if err := setSubscriptionEntity(ctx, l); err != nil {
		return err
	}
	if err := setSubscriptionFilters(ctx, l); err != nil {
		return err
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
		return fmt.Errorf("failed to create new subscription receiver %s: %w", l.subscriptionEntity.Name, err)
	}

	// Create a handle class that has that function
	listenerHandle := subReceiver.Listen(ctx, servicebus.HandlerFunc(
		func(ctx context.Context, msg *servicebus.Message) error {
			originalHandler := handler
			for !message.IsDone(handler) {
				handler = handler.Do(ctx, originalHandler, msg)
				// handle nil as a Completion!
				if handler == nil {
					handler = message.Complete()
				}
			}
			return nil
		}))
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

func getNamespace(option servicebus.NamespaceOption) (*servicebus.Namespace, error) {
	namespace, err := servicebus.NewNamespace(option)
	if err != nil {
		return nil, fmt.Errorf("creating service bus namespace failed: %w", err)
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
		return nil, fmt.Errorf("creating subscription manager failed: %w", err)
	}

	subEntity, err := ensureSubscription(ctx, subscriptionManager, subscriptionName)
	if err != nil {
		return nil, fmt.Errorf("ensuring subscription failed: %w", err)
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
		return fmt.Errorf("listing subscription filter rules failed: %w", err)
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
				return fmt.Errorf("deleting subscription filter rule failed: %w", err)
			}
			_, err = sm.PutRule(ctx, subName, filterName, filter)
			if err != nil {
				return fmt.Errorf("updating subscription filter rule failed: %w", err)
			}
			return nil
		}
	}

	// remove the default rule, which is the "TrueFilter" that accepts all messages
	err = sm.DeleteRule(ctx, subName, "$Default")
	if err != nil {
		return fmt.Errorf("deleting default rule failed: %w", err)
	}
	_, err = sm.PutRule(ctx, subName, filterName, filter)
	if err != nil {
		return fmt.Errorf("updating subscription filter rule failed: %w", err)
	}
	return nil
}

func compareFilter(f1, f2 servicebus.FilterDescription) bool {
	if f1.Type == "CorrelationFilter" {
		return f1.CorrelationFilter.CorrelationID == f2.CorrelationFilter.CorrelationID &&
			f1.CorrelationFilter.Label == f2.CorrelationFilter.Label
	}
	// other than CorrelationFilter all other filters use a SQLExpression
	return *f1.SQLExpression == *f2.SQLExpression
}
