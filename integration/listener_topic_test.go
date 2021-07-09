// +build integration

package integration

import (
	"context"
	"github.com/Azure/go-shuttle/listener/topic"
	"strings"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/message"
)

// TestCreateNewListenerFromConnectionString tests the creation of a listener with a connection string
func (suite *serviceBusTopicSuite) TestCreateNewListener() {
	_, err := topic.New(suite.listenerAuthOption)
	suite.NoError(err)
}

func (suite *serviceBusTopicSuite) TestListenWithDefault() {
	listener, err := topic.New(suite.listenerAuthOption)
	if suite.NoError(err) {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		go func() {
			time.Sleep(10 * time.Second)
			cancel()
		}()
		err := listener.Listen(
			ctx,
			createTestHandler(),
			suite.TopicName,
		)
		suite.True(contextCanceledError(err), "listener listen function failed", err.Error())
		suite.EqualValues(listener.Subscription().Name, "default")
	}
}

func (suite *serviceBusTopicSuite) TestListenWithCustomSubscription() {
	listener, err := topic.New(
		suite.listenerAuthOption,
		topic.WithSubscriptionName("subName"))
	if suite.NoError(err) {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		go func() {
			time.Sleep(10 * time.Second)
			cancel()
		}()
		err := listener.Listen(
			ctx,
			createTestHandler(),
			suite.TopicName,
		)
		suite.True(contextCanceledError(err), "listener listen function failed", err.Error())
		suite.EqualValues(listener.Subscription().Name, "subName")
	}
}

func (suite *serviceBusTopicSuite) TestListenWithCustomFilter() {
	createRule, err := topic.New(
		suite.listenerAuthOption,
		topic.WithSubscriptionName("default"),
		topic.WithFilterDescriber("testFilter",
			servicebus.SQLFilter{Expression: "destinationId LIKE test"}))
	if suite.NoError(err) {
		suite.startListenAndCheckForFilter(
			createRule,
			"testFilter",
			servicebus.SQLFilter{Expression: "destinationId LIKE test"},
		)
		// check if it correctly replaces an existing rule
		replaceRuleSub, err := topic.New(
			suite.listenerAuthOption,
			topic.WithSubscriptionName("default"),
			topic.WithFilterDescriber("testFilter",
				servicebus.SQLFilter{Expression: "destinationId LIKE test2"}))
		if suite.NoError(err) {
			suite.startListenAndCheckForFilter(
				replaceRuleSub,
				"testFilter",
				servicebus.SQLFilter{Expression: "destinationId LIKE test2"},
			)
		}
	}
}

// startListenAndCheckForFilter starts the listener and makes sure that the correct rules were created
func (suite *serviceBusTopicSuite) startListenAndCheckForFilter(
	l *topic.Listener,
	filterName string,
	filter servicebus.FilterDescriber) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	go func() {
		time.Sleep(10 * time.Second)
		cancel()
	}()
	err := l.Listen(
		ctx,
		createTestHandler(),
		suite.TopicName,
	)
	suite.True(contextCanceledError(err), "listener listen function failed", err.Error())
	ruleExists, err := suite.checkRule(l.Namespace(), l.Subscription().Name, filterName, filter)
	if suite.NoError(err) {
		suite.True(ruleExists)
	}
}

func createTestHandler() message.HandleFunc {
	return func(ctx context.Context, message *message.Message) message.Handler {
		return message.Complete()
	}
}

func contextCanceledError(err error) bool {
	return strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "context canceled")
}

func (suite *serviceBusTopicSuite) checkRule(ns *servicebus.Namespace, subName, filterName string, filter servicebus.FilterDescriber) (bool, error) {
	sm, err := ns.NewSubscriptionManager(suite.TopicName)
	if err != nil {
		return false, err
	}
	rules, err := sm.ListRules(context.Background(), subName)
	if err != nil {
		return false, err
	}
	for _, rule := range rules {
		if rule.Name == filterName {
			if compareFilter(rule.Filter, filter.ToFilterDescription()) {
				return true, nil
			}
		}
	}
	return false, nil
}

func compareFilter(f1, f2 servicebus.FilterDescription) bool {
	if f1.Type == "CorrelationFilter" {
		return f1.CorrelationFilter.CorrelationID == f2.CorrelationFilter.CorrelationID &&
			f1.CorrelationFilter.Label == f2.CorrelationFilter.Label
	}
	// other than CorrelationFilter all other filters use a SQLExpression
	return *f1.SQLExpression == *f2.SQLExpression
}
