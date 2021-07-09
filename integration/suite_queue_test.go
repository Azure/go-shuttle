// +build integration

package integration

import (
	"context"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-shuttle/internal/test"
	queue2 "github.com/Azure/go-shuttle/listener/queue"
	"github.com/Azure/go-shuttle/publisher/queue"
	"github.com/stretchr/testify/suite"
	"os"
	"testing"
)

type serviceBusQueueSuite struct {
	test.BaseSuite
	Prefix              string
	QueueName           string
	Publisher           queue.Publisher
	Listener            queue2.Listener
	publisherAuthOption queue.ManagementOption
	listenerAuthOption  queue2.ManagementOption
}

const testQueueName = "testQueue"

func TestQueueConnectionString(t *testing.T) {
	t.Parallel()
	connectionStringSuite := &serviceBusQueueSuite{
		Prefix:              "conn-",
		publisherAuthOption: withQueuePublisherConnectionString(),
	}
	suite.Run(t, connectionStringSuite)
}

func TestQueueClientId(t *testing.T) {
	t.Parallel()
	clientIdSuite := &serviceBusQueueSuite{
		Prefix:              "cid-",
		publisherAuthOption: withQueuePublisherManagedIdentityClientID(),
	}
	suite.Run(t, clientIdSuite)
}

func TestQueueResourceID(t *testing.T) {
	t.Parallel()
	resourceIdSuite := &serviceBusQueueSuite{
		Prefix:              "rid-",
		publisherAuthOption: withQueuePublisherManagedIdentityResourceID(),
	}
	suite.Run(t, resourceIdSuite)
}

func withQueuePublisherConnectionString() queue.ManagementOption {
	connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if connStr == "" {
		panic("environment variable SERVICEBUS_CONNECTION_STRING was not set")
	}

	return queue.WithConnectionString(connStr)
}

func withQueuePublisherManagedIdentityClientID() queue.ManagementOption {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		panic("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	// if managedIdentityClientID is empty then library will assume system assigned managed identity
	managedIdentityClientID := os.Getenv("MANAGED_IDENTITY_CLIENT_ID")

	token, err := adalToken(managedIdentityClientID, adal.NewServicePrincipalTokenFromMSIWithUserAssignedID)
	if err != nil {
		panic(err)
	}
	return queue.WithToken(serviceBusNamespaceName, token)
}

func withQueuePublisherManagedIdentityResourceID() queue.ManagementOption {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		panic("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	managedIdentityResourceID := os.Getenv("MANAGED_IDENTITY_RESOURCE_ID")
	if managedIdentityResourceID == "" {
		panic("environment variable MANAGED_IDENTITY_RESOURCE_ID was not set")
	}
	token, err := adalToken(managedIdentityResourceID, adal.NewServicePrincipalTokenFromMSIWithIdentityResourceID)
	if err != nil {
		panic(err)
	}
	return queue.WithToken(serviceBusNamespaceName, token)
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
	listener         *queue2.Listener
	publisher        *queue.Publisher
	listenerOptions  []queue2.Option
	publisherOptions []queue.Option
	publishCount     *int
	shouldSucceed    bool
}

