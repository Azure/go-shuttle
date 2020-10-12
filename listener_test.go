package pubsub

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/keikumata/azure-pub-sub/message"
)

// TestCreateNewListenerFromConnectionString tests the creation of a listener with a connection string
func (suite *serviceBusSuite) TestCreateNewListenerWithConnectionString() {
	listener, err := createNewListenerWithConnectionString()
	if suite.NoError(err) {
		connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING")
		suite.Contains(connStr, listener.namespace.Name)
	}
}

func (suite *serviceBusSuite) TestListenWithDefault() {
	listener, err := createNewListenerWithConnectionString()
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
		suite.EqualValues(listener.subscriptionEntity.Name, "default")
	}
}

func (suite *serviceBusSuite) TestListenWithCustomSubscription() {
	listener, err := createNewListenerWithConnectionString()
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
			SetSubscriptionName("subName"),
		)
		suite.True(contextCanceledError(err), "listener listen function failed", err.Error())
		suite.EqualValues(listener.subscriptionEntity.Name, "subName")
	}
}

func (suite *serviceBusSuite) TestListenWithCustomFilter() {
	listener, err := createNewListenerWithConnectionString()
	if suite.NoError(err) {
		suite.startListenAndCheckForFilter(
			listener,
			"testFilter",
			servicebus.SQLFilter{Expression: "destinationId LIKE test"},
			"default",
		)
		// check if it correctly replaces an existing rule
		suite.startListenAndCheckForFilter(
			listener,
			"testFilter",
			servicebus.SQLFilter{Expression: "destinationId LIKE test2"},
			"default",
		)
	}
}

// startListenAndCheckForFilter starts the listener and makes sure that the correct rules were created
func (suite *serviceBusSuite) startListenAndCheckForFilter(
	listener *Listener,
	filterName string,
	filter servicebus.FilterDescriber,
	subName string) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	go func() {
		time.Sleep(10 * time.Second)
		cancel()
	}()
	err := listener.Listen(
		ctx,
		createTestHandler(),
		suite.TopicName,
		SetSubscriptionFilter(filterName, filter),
	)
	suite.True(contextCanceledError(err), "listener listen function failed", err.Error())
	ruleExists, err := suite.checkRule(listener.namespace, subName, filterName, filter)
	if suite.NoError(err) {
		suite.True(*ruleExists)
	}
}

func createNewListenerWithConnectionString() (*Listener, error) {
	connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if connStr == "" {
		return nil, errors.New("environment variable SERVICEBUS_CONNECTION_STRING was not set")
	}

	return NewListener(ListenerWithConnectionString(connStr))
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

func (suite *serviceBusSuite) checkRule(ns *servicebus.Namespace, subName, filterName string, filter servicebus.FilterDescriber) (*bool, error) {
	sm, err := ns.NewSubscriptionManager(suite.TopicName)
	if err != nil {
		return nil, err
	}
	rules, err := sm.ListRules(context.Background(), subName)
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		if rule.Name == filterName {
			if compareFilter(rule.Filter, filter.ToFilterDescription()) {
				t := true
				return &t, nil
			}
		}
	}
	f := false
	return &f, nil
}
