package listener

import (
	"context"
	"errors"
	"fmt"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-shuttle/message"
	"github.com/Azure/go-shuttle/peeklock"

	aad "github.com/Azure/go-shuttle/internal/aad"
	servicebusinternal "github.com/Azure/go-shuttle/internal/servicebus"
)

const (
	defaultSubscriptionName = "default"
)

// Listener is a struct to contain service bus entities relevant to subscribing to a publisher topic
type Listener struct {
	namespace           *servicebus.Namespace
	topicEntity         *servicebus.TopicEntity
	subscriptionEntity  *servicebus.SubscriptionEntity
	listenerHandle      *servicebus.ListenerHandle
	topicName           string
	subscriptionName    string
	maxDeliveryCount    int32
	lockRenewalInterval *time.Duration
	lockDuration        time.Duration
	filterDefinitions   []*filterDefinition
}

// Subscription returns the servicebus.SubscriptionEntity that the listener is setup with
func (l *Listener) Subscription() *servicebus.SubscriptionEntity {
	return l.subscriptionEntity
}

// Topic returns servicebus.TopicEntity that the listener is setup with
func (l *Listener) Topic() *servicebus.TopicEntity {
	return l.topicEntity
}

// Namespace returns the servicebus.Namespace that the listener is setup with
func (l *Listener) Namespace() *servicebus.Namespace {
	return l.namespace
}

// ManagementOption provides structure for configuring a new Listener
type ManagementOption func(l *Listener) error

// Option provides structure for configuring when starting to listen to a specified topic
type Option func(l *Listener) error

// WithConnectionString configures a listener with the information provided in a Service Bus connection string
func WithConnectionString(connStr string) ManagementOption {
	return func(l *Listener) error {
		if connStr == "" {
			return errors.New("no Service Bus connection string provided")
		}
		ns, err := servicebus.NewNamespace(servicebus.NamespaceWithConnectionString(connStr))
		if err != nil {
			return err
		}
		l.namespace = ns
		return nil
	}
}

// WithManagedIdentityClientID configures a listener with the attached managed identity and the Service bus resource name
func WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID string) ManagementOption {
	return func(l *Listener) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		ns, err := servicebus.NewNamespace(servicebusinternal.NamespaceWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID))
		if err != nil {
			return err
		}
		l.namespace = ns
		return nil
	}
}

// WithToken configures a listener with a AAD token
func WithToken(serviceBusNamespaceName string, spt *adal.ServicePrincipalToken) ManagementOption {
	return func(l *Listener) error {
		if spt == nil {
			return errors.New("cannot provide a nil token")
		}
		ns, err := servicebus.NewNamespace(servicebusinternal.NamespaceWithTokenProvider(serviceBusNamespaceName, aad.AsJWTTokenProvider(spt)))
		if err != nil {
			return err
		}
		l.namespace = ns
		return nil
	}
}

// WithManagedIdentityResourceID configures a listener with the attached managed identity and the Service bus resource name
func WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) ManagementOption {
	return func(l *Listener) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		ns, err := servicebus.NewNamespace(servicebusinternal.NamespaceWithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID))
		if err != nil {
			return err
		}
		l.namespace = ns
		return nil
	}
}

// WithSubscriptionName configures the subscription name of the subscription to listen to
func WithSubscriptionName(name string) ManagementOption {
	return func(l *Listener) error {
		l.subscriptionName = name
		return nil
	}
}

// WithFilterDescriber configures the filters on the subscription
func WithFilterDescriber(filterName string, filter servicebus.FilterDescriber) ManagementOption {
	return func(l *Listener) error {
		if len(filterName) == 0 || filter == nil {
			return errors.New("filter name or filter cannot be zero value")
		}
		l.filterDefinitions = append(l.filterDefinitions, &filterDefinition{filterName, filter})
		return nil
	}
}

// WithSubscriptionDetails allows listeners to control subscription details for longer lived operations.
// If you using RetryLater you probably want this. Passing zeros leaves it up to Service bus defaults
func WithSubscriptionDetails(lock time.Duration, maxDelivery int32) ManagementOption {
	return func(l *Listener) error {
		if lock > servicebusinternal.LockDuration {
			//working on getting service bus to enforce this. Hangs if you go higher. https://github.com/Azure/azure-service-bus-go/pull/202
			return fmt.Errorf("Lock duration must be <= to %v", servicebusinternal.LockDuration)
		}
		if lock < time.Duration(0) {
			return fmt.Errorf("Lock duration must be positive")
		}
		l.lockDuration = lock
		if maxDelivery < 0 {
			return fmt.Errorf("Max Deliveries must be positive")
		}
		l.maxDeliveryCount = maxDelivery
		return nil
	}
}

func WithMessageLockAutoRenewal(interval time.Duration) Option {
	return func(l *Listener) error {
		if interval < time.Duration(0) {
			return fmt.Errorf("renewal interval must be positive")
		}
		l.lockRenewalInterval = &interval
		return nil
	}
}

type filterDefinition struct {
	Name   string
	Filter servicebus.FilterDescriber
}

// New creates a new service bus listener
func New(opts ...ManagementOption) (*Listener, error) {
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
	if listener.subscriptionName == "" {
		listener.subscriptionName = defaultSubscriptionName
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
	subscriptionEntity, err := l.getSubscriptionEntity(ctx, l.subscriptionName)
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
func (l *Listener) Listen(ctx context.Context, handler message.Handler, topicName string, opts ...Option) error {
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
			if l.lockRenewalInterval != nil {
				renewer := peeklock.RenewPeriodically(ctx, *l.lockRenewalInterval, sub, msg)
				defer renewer.Stop()
			}
			currentHandler := handler
			for !message.IsDone(currentHandler) {
				currentHandler = currentHandler.Do(ctx, handler, msg)
				// handle nil as a Completion!
				if currentHandler == nil {
					currentHandler = message.Complete()
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

// GetActiveMessageCount gets the active message count of a topic subscription
// WARNING: GetActiveMessageCount is 10 times expensive than a call to receive a message
func (l *Listener) GetActiveMessageCount(ctx context.Context, topicName, subscriptionName string) (int32, error) {
	if l.topicEntity == nil {
		return 0, fmt.Errorf("entity of topic is nil")
	}
	if l.topicName != topicName {
		return 0, fmt.Errorf("topic name %q doesn't match %q", topicName, l.topicName)
	}

	subscriptionEntity, err := l.getSubscriptionEntity(ctx, subscriptionName)
	if err != nil {
		return 0, fmt.Errorf("error to get entity of subscription %q of topic %q: %s", subscriptionName, topicName, err)
	}
	if subscriptionEntity == nil {
		return 0, fmt.Errorf("entity of subscription %q of topic %q returned is nil", subscriptionName, topicName)
	}
	if subscriptionEntity.CountDetails == nil || subscriptionEntity.CountDetails.ActiveMessageCount == nil {
		return 0, fmt.Errorf("active message count is not available in the entity of subscription %q of topic %q", subscriptionName, topicName)
	}

	return *subscriptionEntity.CountDetails.ActiveMessageCount, nil
}

func getTopicEntity(ctx context.Context, topicName string, namespace *servicebus.Namespace) (*servicebus.TopicEntity, error) {
	tm := namespace.NewTopicManager()
	return tm.Get(ctx, topicName)
}

func (l *Listener) getSubscriptionEntity(
	ctx context.Context,
	subscriptionName string) (*servicebus.SubscriptionEntity, error) {

	subscriptionManager, err := l.namespace.NewSubscriptionManager(l.topicEntity.Name)
	if err != nil {
		return nil, fmt.Errorf("creating subscription manager failed: %w", err)
	}

	subEntity, err := l.ensureSubscription(ctx, subscriptionManager, subscriptionName)
	if err != nil {
		return nil, fmt.Errorf("ensuring subscription failed: %w", err)
	}

	return subEntity, nil
}

func (l *Listener) ensureSubscription(ctx context.Context, sm *servicebus.SubscriptionManager, name string) (*servicebus.SubscriptionEntity, error) {
	subEntity, err := sm.Get(ctx, name)
	if err == nil {
		return subEntity, nil
	}
	mutateSericeDetails := func(s *servicebus.SubscriptionDescription) error {
		if l.maxDeliveryCount > 0 {
			s.MaxDeliveryCount = &l.maxDeliveryCount
		}
		if l.lockDuration > time.Duration(0) {
			return servicebus.SubscriptionWithLockDuration(&l.lockDuration)(s)
		}
		return nil
	}
	return sm.Put(ctx, name, mutateSericeDetails)
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
