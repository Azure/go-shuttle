package listener

import (
	"context"
	"errors"
	"fmt"
	"time"

	amqp "github.com/Azure/azure-amqp-common-go/v3"
	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/common"
	"github.com/Azure/go-shuttle/handlers"
	"github.com/Azure/go-shuttle/message"
	"github.com/devigned/tab"
)

const (
	defaultSubscriptionName = "default"
)

var _ common.Listener = &Listener{}

type TopicListener interface {
	common.Listener
	SetSubscriptionName(subscriptionName string)
	AppendFilterDefinition(definition *filterDefinition)
}

// Listener is a struct to contain service bus entities relevant to subscribing to a publisher topic
type Listener struct {
	common.ListenerSettings
	topicEntity        *servicebus.TopicEntity
	subscriptionEntity *servicebus.SubscriptionEntity
	listenerHandle     *servicebus.ListenerHandle
	topicName          string
	subscriptionName   string
	filterDefinitions  []*filterDefinition
}

// Subscription returns the servicebus.SubscriptionEntity that the listener is setup with
func (l *Listener) Subscription() *servicebus.SubscriptionEntity {
	return l.subscriptionEntity
}

// Topic returns servicebus.TopicEntity that the listener is setup with
func (l *Listener) Topic() *servicebus.TopicEntity {
	return l.topicEntity
}

func (l *Listener) SetSubscriptionName(subscriptionName string) {
	l.subscriptionName = subscriptionName
}

func (l *Listener) AppendFilterDefinition(definition *filterDefinition) {
	l.filterDefinitions = append(l.filterDefinitions, definition)
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
	listener := &Listener{
		ListenerSettings: common.ListenerSettings{},
	}
	listener.SetNamespace(ns)
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
	topicEntity, err := getTopicEntity(ctx, l.topicName, l.Namespace())
	if err != nil {
		return fmt.Errorf("failed to get topic: %w", err)
	}
	l.topicEntity = topicEntity
	return nil
}

func setSubscriptionFilters(ctx context.Context, l *Listener) error {
	if len(l.filterDefinitions) == 0 {
		return nil
	}
	sm, err := l.Namespace().NewSubscriptionManager(l.topicEntity.Name)
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
	ctx, span := tab.StartSpan(ctx, "go-shuttle.listener.Listen")
	defer span.End()
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
	if err := ensureSubscriptionEntity(ctx, l); err != nil {
		return err
	}
	if err := setSubscriptionFilters(ctx, l); err != nil {
		return err
	}
	// Generate new topic client
	topic, err := l.Namespace().NewTopic(l.topicEntity.Name)
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

	var receiverOpts []servicebus.ReceiverOption
	if l.PrefetchCount() != nil {
		receiverOpts = append(receiverOpts, servicebus.ReceiverWithPrefetchCount(*l.PrefetchCount()))
	}
	handlerConcurrency := 1
	if l.MaxConcurrency() != nil {
		handlerConcurrency = *l.MaxConcurrency()
	}
	subReceiver, err := sub.NewReceiver(ctx, receiverOpts...)
	if err != nil {
		return fmt.Errorf("failed to create new subscription receiver %s: %w", l.subscriptionEntity.Name, err)
	}
	listenerHandle := subReceiver.Listen(ctx,
		handlers.NewConcurrent(handlerConcurrency,
			handlers.NewDeadlineContext(
				handlers.NewPeekLockRenewer(l.LockRenewalInterval(), sub,
					handlers.NewShuttleAdapter(handler)),
			)))
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

func ensureSubscriptionEntity(ctx context.Context, l *Listener) error {
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

func (l *Listener) getSubscriptionEntity(
	ctx context.Context,
	subscriptionName string) (*servicebus.SubscriptionEntity, error) {

	subscriptionManager, err := l.Namespace().NewSubscriptionManager(l.topicEntity.Name)
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
	attempt := 1
	ensure := func() (interface{}, error) {
		ctx, span := tab.StartSpan(ctx, "go-shuttle.listener.ensureSubscription")
		span.AddAttributes(tab.Int64Attribute("retry.attempt", int64(attempt)))
		defer span.End()
		subEntity, err := sm.Get(ctx, name)
		if err == nil {
			return subEntity, nil
		}
		mutateSericeDetails := func(s *servicebus.SubscriptionDescription) error {
			maxDeliveryCount := l.MaxDeliveryCount()
			if maxDeliveryCount > 0 {
				s.MaxDeliveryCount = &maxDeliveryCount
			}
			lockDuration := l.LockDuration()
			if lockDuration > time.Duration(0) {
				return servicebus.SubscriptionWithLockDuration(&lockDuration)(s)
			}
			return nil
		}
		entity, err := sm.Put(ctx, name, mutateSericeDetails)
		if err != nil {
			attempt++
			tab.For(ctx).Error(err)
			// let all errors be retryable for now. application only hit this once on subscription creation.
			return nil, amqp.Retryable(err.Error())
		}
		return entity, err
	}
	entity, err := amqp.Retry(5, 1*time.Second, ensure)
	if err != nil {
		tab.For(ctx).Error(err)
		return nil, err
	}
	return entity.(*servicebus.SubscriptionEntity), nil
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
