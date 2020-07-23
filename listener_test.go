package pubsub

import (
	"context"
	"errors"
	"os"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
)

// TestCreateNewListenerFromConnectionString tests the creation of a listener with a connection string
func (suite *serviceBusSuite) TestCreateNewListenerFromConnectionString() {
	listener, err := createNewListenerFromConnectionString()
	if suite.NoError(err) {
		connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING")
		suite.Contains(connStr, listener.namespace.Name)
	}
}

func (suite *serviceBusSuite) TestListenWithDefault() {
	listener, err := createNewListenerFromConnectionString()
	if suite.NoError(err) {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		go func() {
			time.Sleep(10 * time.Second)
			cancel()
		}()
		listener.Listen(
			ctx,
			createTestHandler(),
			suite.TopicName,
		)
		suite.EqualValues(listener.subscriptionEntity.Name, "default")
	}
}

func (suite *serviceBusSuite) TestListenWithCustomSubscription() {
	listener, err := createNewListenerFromConnectionString()
	if suite.NoError(err) {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		go func() {
			time.Sleep(10 * time.Second)
			cancel()
		}()
		listener.Listen(
			ctx,
			createTestHandler(),
			suite.TopicName,
			SetSubscriptionName("subName"),
		)
		suite.EqualValues(listener.subscriptionEntity.Name, "subName")
	}
}

func (suite *serviceBusSuite) TestListenWithCustomFilter() {
	listener, err := createNewListenerFromConnectionString()
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
	listener.Listen(
		ctx,
		createTestHandler(),
		suite.TopicName,
		SetSubscriptionFilter(filterName, filter),
	)
	ruleExists, err := suite.checkRule(subName, filterName, filter)
	if suite.NoError(err) {
		suite.True(*ruleExists)
	}
}

func createNewListenerFromConnectionString() (*Listener, error) {
	connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if connStr == "" {
		return nil, errors.New("environment variable SERVICEBUS_CONNECTION_STRING was not set")
	}

	return NewListener(ListenerWithConnectionString(connStr))
}

func createTestHandler() Handle {
	return func(ctx context.Context, message string, messageType string) error {
		return nil
	}
}

func (suite *serviceBusSuite) checkRule(subName, filterName string, filter servicebus.FilterDescriber) (*bool, error) {
	ns := suite.GetNewNamespace()
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
