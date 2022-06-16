//go:build integration
// +build integration

package integration

import (
	"context"
	"os"
	"testing"

	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/stretchr/testify/suite"

	"github.com/Azure/go-shuttle/internal/test"
	"github.com/Azure/go-shuttle/queue/listener"
	"github.com/Azure/go-shuttle/queue/publisher"
)

type serviceBusQueueSuite struct {
	test.BaseSuite
	Prefix              string
	QueueName           string
	Publisher           publisher.Publisher
	Listener            listener.Listener
	publisherAuthOption publisher.ManagementOption
	listenerAuthOption  listener.ManagementOption
}

const testQueueName = "testQueue"

func TestQueueConnectionString(t *testing.T) {
	t.Parallel()
	connectionStringSuite := &serviceBusQueueSuite{
		Prefix:              "conn-",
		listenerAuthOption:  withListenerConnectionString(),
		publisherAuthOption: withQueuePublisherConnectionString(),
	}
	suite.Run(t, connectionStringSuite)
}

func TestQueueClientId(t *testing.T) {
	t.Parallel()
	clientIdSuite := &serviceBusQueueSuite{
		Prefix:              "cid-",
		listenerAuthOption:  withListenerManagedIdentityClientID(),
		publisherAuthOption: withQueuePublisherManagedIdentityClientID(),
	}
	suite.Run(t, clientIdSuite)
}

func TestQueueResourceID(t *testing.T) {
	t.Parallel()
	resourceIdSuite := &serviceBusQueueSuite{
		Prefix:              "rid-",
		listenerAuthOption:  withListenerManagedIdentityResourceID(),
		publisherAuthOption: withQueuePublisherManagedIdentityResourceID(),
	}
	suite.Run(t, resourceIdSuite)
}

func withQueuePublisherConnectionString() publisher.ManagementOption {
	connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if connStr == "" {
		panic("environment variable SERVICEBUS_CONNECTION_STRING was not set")
	}

	return publisher.WithConnectionString(connStr)
}

func withQueuePublisherManagedIdentityClientID() publisher.ManagementOption {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		panic("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	// if managedIdentityClientID is empty then library will assume system assigned managed identity
	managedIdentityClientID := os.Getenv("MANAGED_IDENTITY_CLIENT_ID")

	spt, err := adalToken(&adal.ManagedIdentityOptions{
		ClientID: managedIdentityClientID,
	})
	if err != nil {
		panic(err)
	}
	return publisher.WithToken(serviceBusNamespaceName, spt)
}

func withQueuePublisherManagedIdentityResourceID() publisher.ManagementOption {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		panic("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	managedIdentityResourceID := os.Getenv("MANAGED_IDENTITY_RESOURCE_ID")
	if managedIdentityResourceID == "" {
		panic("environment variable MANAGED_IDENTITY_RESOURCE_ID was not set")
	}
	token, err := adalToken(&adal.ManagedIdentityOptions{
		IdentityResourceID: managedIdentityResourceID,
	})
	if err != nil {
		panic(err)
	}
	return publisher.WithToken(serviceBusNamespaceName, token)
}

func (suite *serviceBusQueueSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
	suite.QueueName = suite.Prefix + testQueueName + suite.TagID
	_, err := suite.EnsureQueue(context.Background(), suite.QueueName)
	if err != nil {
		suite.T().Fatal(err)
	}
}

type publishReceiveQueueTest struct {
	queueName        string
	listener         *listener.Listener
	publisher        *publisher.Publisher
	listenerOptions  []listener.Option
	publisherOptions []publisher.Option
	publishCount     *int
	shouldSucceed    bool
}
