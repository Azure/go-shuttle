package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-shuttle/internal/test"
	"github.com/Azure/go-shuttle/listener"
	"github.com/Azure/go-shuttle/publisher"
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
		listenerAuthOption:  withListenerConnectionString(),
		publisherAuthOption: withPublisherConnectionString(),
	}
	suite.Run(t, connectionStringSuite)
}

func TestClientId(t *testing.T) {
	t.Parallel()
	clientIdSuite := &serviceBusSuite{
		Prefix:              "cid-",
		listenerAuthOption:  withListenerManagedIdentityClientID(),
		publisherAuthOption: withPublisherManagedIdentityClientID(),
	}
	suite.Run(t, clientIdSuite)
}

func TestResourceID(t *testing.T) {
	t.Parallel()
	resourceIdSuite := &serviceBusSuite{
		Prefix:              "rid-",
		listenerAuthOption:  withListenerManagedIdentityResourceID(),
		publisherAuthOption: withPublisherManagedIdentityResourceID(),
	}
	suite.Run(t, resourceIdSuite)
}

func withListenerConnectionString() listener.ManagementOption {
	connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if connStr == "" {
		panic("environment variable SERVICEBUS_CONNECTION_STRING was not set")
	}
	return listener.WithConnectionString(connStr)
}

func withPublisherConnectionString() publisher.ManagementOption {
	connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if connStr == "" {
		panic("environment variable SERVICEBUS_CONNECTION_STRING was not set")
	}

	return publisher.WithConnectionString(connStr)
}

func withListenerManagedIdentityClientID() listener.ManagementOption {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		panic("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}
	// if managedIdentityClientID is empty then library will assume system assigned managed identity
	managedIdentityClientID := os.Getenv("MANAGED_IDENTITY_CLIENT_ID")
	if managedIdentityClientID == "" {
		panic("environment variable MANAGED_IDENTITY_CLIENT_ID was not set")
	}
	spt, err := adalToken(managedIdentityClientID, adal.NewServicePrincipalTokenFromMSIWithUserAssignedID)
	if err != nil {
		panic(err.Error())
	}
	return listener.WithToken(serviceBusNamespaceName, spt)
}

func withListenerManagedIdentityResourceID() listener.ManagementOption {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		panic("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	// if managedIdentityClientID is empty then library will assume system assigned managed identity
	managedIdentityResourceID := os.Getenv("MANAGED_IDENTITY_RESOURCE_ID")
	if managedIdentityResourceID == "" {
		panic("environment variable MANAGED_IDENTITY_RESOURCE_ID was not set")
	}

	spt, err := adalToken(managedIdentityResourceID, adal.NewServicePrincipalTokenFromMSIWithIdentityResourceID)
	if err != nil {
		panic(err)
	}
	return listener.WithToken(serviceBusNamespaceName, spt)
}

type withSpecificIdFunc func(msiEndpoint, resource string, identityResourceID string, callbacks ...adal.TokenRefreshCallback) (*adal.ServicePrincipalToken, error)

func adalToken(id string, getToken withSpecificIdFunc) (*adal.ServicePrincipalToken, error) {
	msiEndpoint, err := adal.GetMSIVMEndpoint()
	if err != nil {
		return nil, err
	}
	logrefresh := func(t adal.Token) error {
		fmt.Printf("refreshing token: %s", t.Expires())
		return nil
	}
	spt, err := getToken(msiEndpoint, serviceBusResourceURI, id, logrefresh)
	if err != nil {
		return nil, err
	}
	return spt, nil
}

func withPublisherManagedIdentityClientID() publisher.ManagementOption {
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
	return publisher.WithToken(serviceBusNamespaceName, token)
}

func withPublisherManagedIdentityResourceID() publisher.ManagementOption {
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
	return publisher.WithToken(serviceBusNamespaceName, token)
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
