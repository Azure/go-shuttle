package integration

import (
	"context"
	"strings"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/listener"
	"github.com/Azure/go-shuttle/message"
)

// TestCreateNewListenerFromConnectionString tests the creation of a listener with a connection string
func (suite *serviceBusSuite) TestCreateNewListener() {
	_, err := listener.New(suite.listenerAuthOption)
	suite.NoError(err)
}

func (suite *serviceBusSuite) TestListenWithDefault() {
	listener, err := listener.New(suite.listenerAuthOption)
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

func (suite *serviceBusSuite) TestListenWithCustomSubscription() {
	listener, err := listener.New(
		suite.listenerAuthOption,
		listener.WithSubscriptionName("subName"))
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

func (suite *serviceBusSuite) TestListenWithCustomFilter() {
	createRule, err := listener.New(
		suite.listenerAuthOption,
		listener.WithSubscriptionName("default"),
		listener.WithFilterDescriber("testFilter",
			servicebus.SQLFilter{Expression: "destinationId LIKE test"}))
	if suite.NoError(err) {
		suite.startListenAndCheckForFilter(
			createRule,
			"testFilter",
			servicebus.SQLFilter{Expression: "destinationId LIKE test"},
		)
		// check if it correctly replaces an existing rule
		replaceRuleSub, err := listener.New(
			suite.listenerAuthOption,
			listener.WithSubscriptionName("default"),
			listener.WithFilterDescriber("testFilter",
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
func (suite *serviceBusSuite) startListenAndCheckForFilter(
	l *listener.Listener,
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

func (suite *serviceBusSuite) checkRule(ns *servicebus.Namespace, subName, filterName string, filter servicebus.FilterDescriber) (bool, error) {
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
