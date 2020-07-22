// +build !withconnectionstring

package pubsub

import (
	"context"
	"errors"
	"os"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
)

// TestCreateNewListenerFromConnectionString tests the creation of a listener with a connection string
func (suite *serviceBusSuite) TestCreateNewListenerWithManagedIdentity() {
	listener, err := createNewListenerWithManagedIdentity()
	if suite.NoError(err) {
		serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME")
		suite.Contains(serviceBusNamespaceName, listener.namespace.Name)
	}
}

func (suite *serviceBusSuite) TestListenWithDefaultUsingManagedIdentity() {
	listener, err := createNewListenerWithManagedIdentity()
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
		suite.True(err == nil || contextCanceledError(err), "listener listen function failed", err.Error())
		suite.EqualValues(listener.subscriptionEntity.Name, "default")
	}
}

func (suite *serviceBusSuite) TestListenWithCustomFilterUsingManagedIdentity() {
	listener, err := createNewListenerWithManagedIdentity()
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

func createNewListenerWithManagedIdentity() (*Listener, error) {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		return nil, errors.New("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	// if managedIdentityClientID is empty then library will assume system assigned managed identity
	managedIdentityClientID := os.Getenv("MANAGED_IDENTITY_CLIENT_ID")

	return NewListener(ListenerWithManagedIdentity(serviceBusNamespaceName, managedIdentityClientID))
}
