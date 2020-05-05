package pubsub

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/joho/godotenv"
	"github.com/keikumata/azure-pub-sub/internal/test"
	"github.com/stretchr/testify/suite"
)

type (
	serviceBusSuite struct {
		test.BaseSuite
	}
)

const (
	defaultTimeout = 60 * time.Second
	topicName      = "testTopic"
)

func TestSB(t *testing.T) {
	suite.Run(t, new(serviceBusSuite))
}

func (suite *serviceBusSuite) SetupTest() {
	_, err := suite.EnsureTopic(context.Background(), topicName+suite.TagID)
	if err != nil {
		suite.T().Fatal(err)
	}
}

// TestCreateNewListenerFromConnectionString tests the creation of a listener with a connection string
func (suite *serviceBusSuite) TestCreateNewListenerFromConnectionString() {
	_ = godotenv.Load()

	listener, err := createNewListenerFromConnectionString()
	if suite.NoError(err) {
		connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING")
		suite.Contains(connStr, listener.namespace.Name)
	}
}

func (suite *serviceBusSuite) TestListenWithDefault() {
	_ = godotenv.Load()
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
			topicName+suite.TagID,
		)
		suite.EqualValues(listener.subscriptionEntity.Name, "default")
	}
}

func (suite *serviceBusSuite) TestListenWithCustomSubscription() {
	_ = godotenv.Load()
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
			topicName+suite.TagID,
			SetSubscriptionName("subName"),
		)
		suite.EqualValues(listener.subscriptionEntity.Name, "subName")
	}
}

func (suite *serviceBusSuite) TestListenWithCustomFilter() {
	_ = godotenv.Load()
	listener, err := createNewListenerFromConnectionString()
	if suite.NoError(err) {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
		go func() {
			time.Sleep(10 * time.Second)
			cancel()
		}()
		listener.Listen(
			ctx,
			createTestHandler(),
			topicName+suite.TagID,
			SetSubscriptionFilter(
				"testFilter",
				servicebus.SQLFilter{Expression: "destinationId LIKE test"},
			),
		)
		ruleExists, err := suite.checkRule("default", "testFilter")
		if suite.NoError(err) {
			suite.True(*ruleExists)
		}
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
	return func(ctx context.Context, message string) error {
		return nil
	}
}

func (suite *serviceBusSuite) checkRule(subName, filterName string) (*bool, error) {
	ns := suite.GetNewNamespace()
	sm, err := ns.NewSubscriptionManager(topicName + suite.TagID)
	if err != nil {
		return nil, err
	}
	rules, err := sm.ListRules(context.Background(), subName)
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		if rule.Name == filterName {
			t := true
			return &t, nil
		}
	}
	f := false
	return &f, nil
}
