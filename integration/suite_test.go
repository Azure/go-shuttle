package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/keikumata/azure-pub-sub/internal/aad"
	"github.com/keikumata/azure-pub-sub/internal/test"
	"github.com/keikumata/azure-pub-sub/listener"
	"github.com/keikumata/azure-pub-sub/publisher"
	"github.com/stretchr/testify/suite"
)

type serviceBusSuite struct {
	test.BaseSuite
	Prefix              string
	TopicName           string
	Publisher           publisher.Publisher
	Listener            listener.Listener
	publisherAuthOption publisher.ManagementOption
	listenerAuthOption  listener.ManagementOption
}

type retryLaterEvent struct {
	ID    int    `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type testEvent struct {
	ID    int    `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

const (
	defaultTimeout        = 60 * time.Second
	testTopicName         = "testTopic"
	serviceBusResourceURI = "https://servicebus.azure.net/"
)

func TestConnectionString(t *testing.T) {
	t.Parallel()
	connectionStringSuite := &serviceBusSuite{
		Prefix:              "conn-",
		listenerAuthOption:  withEnvConnectionString(),
		publisherAuthOption: withEnvPublisherConnectionString(),
	}
	suite.Run(t, connectionStringSuite)
}

func TestClientId(t *testing.T) {
	t.Parallel()
	clientIdSuite := &serviceBusSuite{
		Prefix:              "cid-",
		listenerAuthOption:  withEnvManagedIdentityClientID(),
		publisherAuthOption: withPublisherEnvManagedIdentityClientID(),
	}
	suite.Run(t, clientIdSuite)
}

func TestResourceID(t *testing.T) {
	t.Parallel()
	resourceIdSuite := &serviceBusSuite{
		Prefix:              "rid-",
		listenerAuthOption:  withEnvManagedIdentityResourceID(),
		publisherAuthOption: withPublisherEnvManagedIdentityResourceID(),
	}
	suite.Run(t, resourceIdSuite)
}

func withEnvConnectionString() listener.ManagementOption {
	connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if connStr == "" {
		panic("environment variable SERVICEBUS_CONNECTION_STRING was not set")
	}
	return listener.WithConnectionString(connStr)
}

func withEnvPublisherConnectionString() publisher.ManagementOption {
	connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if connStr == "" {
		panic("environment variable SERVICEBUS_CONNECTION_STRING was not set")
	}

	return publisher.WithConnectionString(connStr)
}

func withEnvManagedIdentityClientID() listener.ManagementOption {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		panic("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	// if managedIdentityClientID is empty then library will assume system assigned managed identity
	managedIdentityClientID := os.Getenv("MANAGED_IDENTITY_CLIENT_ID")
	if managedIdentityClientID == "" {
		panic("environment variable MANAGED_IDENTITY_CLIENT_ID was not set")
	}
	provider, err := aad.NewJWTProvider(
		aad.JWTProviderWithManagedIdentityClientID(managedIdentityClientID, ""),
		aad.JWTProviderWithResourceURI(serviceBusResourceURI))
	if err != nil {
		panic(err.Error())
	}
	return listener.WithTokenProvider(serviceBusNamespaceName, provider)
}

func withEnvManagedIdentityResourceID() listener.ManagementOption {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		panic("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	// if managedIdentityClientID is empty then library will assume system assigned managed identity
	managedIdentityResourceID := os.Getenv("MANAGED_IDENTITY_RESOURCE_ID")
	if managedIdentityResourceID == "" {
		panic("environment variable MANAGED_IDENTITY_RESOURCE_ID was not set")
	}

	provider, err := aad.NewJWTProvider(
		aad.JWTProviderWithManagedIdentityResourceID(managedIdentityResourceID, ""),
		aad.JWTProviderWithResourceURI(serviceBusResourceURI))
	if err != nil {
		panic(err.Error())
	}
	return listener.WithTokenProvider(serviceBusNamespaceName, provider)
}

func withPublisherEnvManagedIdentityClientID() publisher.ManagementOption {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		panic("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	// if managedIdentityClientID is empty then library will assume system assigned managed identity
	managedIdentityClientID := os.Getenv("MANAGED_IDENTITY_CLIENT_ID")

	provider, err := aad.NewJWTProvider(
		aad.JWTProviderWithManagedIdentityClientID(managedIdentityClientID, ""),
		aad.JWTProviderWithResourceURI(serviceBusResourceURI))
	if err != nil {
		panic(err.Error())
	}
	return publisher.WithTokenProvider(serviceBusNamespaceName, provider)
}

func withPublisherEnvManagedIdentityResourceID() publisher.ManagementOption {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		panic("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	managedIdentityResourceID := os.Getenv("MANAGED_IDENTITY_RESOURCE_ID")
	if managedIdentityResourceID == "" {
		panic("environment variable MANAGED_IDENTITY_RESOURCE_ID was not set")
	}

	provider, err := aad.NewJWTProvider(
		aad.JWTProviderWithManagedIdentityResourceID(managedIdentityResourceID, ""),
		aad.JWTProviderWithResourceURI(serviceBusResourceURI))
	if err != nil {
		panic(err.Error())
	}
	return publisher.WithTokenProvider(serviceBusNamespaceName, provider)
}

func (suite *serviceBusSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
	suite.TopicName = suite.Prefix + testTopicName + suite.TagID
	_, err := suite.EnsureTopic(context.Background(), suite.TopicName)
	if err != nil {
		suite.T().Fatal(err)
	}
}

type publishReceiveTest struct {
	topicName        string
	listener         *listener.Listener
	publisher        *publisher.Publisher
	listenerOptions  []listener.Option
	publisherOptions []publisher.Option
	publishCount     *int
	shouldSucceed    bool
}
