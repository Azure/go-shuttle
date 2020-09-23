// +build !withconnectionstring

package pubsub

import (
	"context"
	"errors"
	"os"
	"time"
)

// TestCreatePublisherWithManagedIdentityWithNewTopic tests the creation of a publisher for a new topic and managed identity
func (suite *serviceBusSuite) TestCreatePublisherWithManagedIdentityUsingNewTopic() {
	topicName := "newTopic" + suite.TagID
	publisher, err := createNewPublisherWithManagedIdentity(topicName)
	if suite.NoError(err) {
		serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME")
		suite.Contains(serviceBusNamespaceName, publisher.namespace.Name)

		// make sure that topic exists
		ns := suite.GetNewNamespace()
		tm := ns.NewTopicManager()
		_, err := tm.Get(context.Background(), topicName)
		suite.NoError(err)

		// delete new topic
		err = tm.Delete(context.Background(), topicName)
		suite.NoError(err)
	}
}

// TestCreatePublisherWithManagedIdentityWithNewTopic tests the creation of a publisher for a new topic and managed identity
func (suite *serviceBusSuite) TestCreatePublisherWithManagedIdentityResourceIDUsingNewTopic() {
	topicName := "newTopic" + suite.TagID
	publisher, err := createNewPublisherWithManagedIdentityResourceID(topicName)
	if suite.NoError(err) {
		serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME")
		suite.Contains(serviceBusNamespaceName, publisher.namespace.Name)

		// make sure that topic exists
		ns := suite.GetNewNamespace()
		tm := ns.NewTopicManager()
		_, err := tm.Get(context.Background(), topicName)
		suite.NoError(err)

		// delete new topic
		err = tm.Delete(context.Background(), topicName)
		suite.NoError(err)
	}
}

// TestCreatePublisherFromConnectionStringWithExistingTopic tests the creation of a publisher for an existing topic and a connection string
func (suite *serviceBusSuite) TestCreatePublisherWithManagedIdentityUsingExistingTopic() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithManagedIdentity(suite.TopicName)
	if suite.NoError(err) {
		serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME")
		suite.Contains(serviceBusNamespaceName, publisher.namespace.Name)

		// make sure that topic exists
		ns := suite.GetNewNamespace()
		tm := ns.NewTopicManager()
		_, err := tm.Get(context.Background(), suite.TopicName)
		suite.NoError(err)
	}
}

func createNewPublisherWithManagedIdentity(topicName string) (*Publisher, error) {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		return nil, errors.New("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	// if managedIdentityClientID is empty then library will assume system assigned managed identity
	managedIdentityClientID := os.Getenv("MANAGED_IDENTITY_CLIENT_ID")

	return NewPublisher(topicName, PublisherWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID))
}

func createNewPublisherWithManagedIdentityResourceID(topicName string) (*Publisher, error) {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		return nil, errors.New("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	// if managedIdentityClientID is empty then library will assume system assigned managed identity
	managedIdentityResourceID := os.Getenv("MANAGED_IDENTITY_RESOURCE_ID")

	return NewPublisher(topicName, PublisherWithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID))
}

func createNewPublisherWithManagedIdentityUsingCustomHeader(topicName, headerName, msgKey string) (*Publisher, error) {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		return nil, errors.New("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	// if managedIdentityClientID is empty then library will assume system assigned managed identity
	managedIdentityClientID := os.Getenv("MANAGED_IDENTITY_CLIENT_ID")

	return NewPublisher(
		topicName,
		PublisherWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID),
		SetDefaultHeader(headerName, msgKey),
	)
}

func createNewPublisherWithManagedIdentityUsingDuplicateDetection(topicName string, window *time.Duration) (*Publisher, error) {
	serviceBusNamespaceName := os.Getenv("SERVICEBUS_NAMESPACE_NAME") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if serviceBusNamespaceName == "" {
		return nil, errors.New("environment variable SERVICEBUS_NAMESPACE_NAME was not set")
	}

	// if managedIdentityClientID is empty then library will assume system assigned managed identity
	managedIdentityClientID := os.Getenv("MANAGED_IDENTITY_CLIENT_ID")

	return NewPublisher(
		topicName,
		PublisherWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID),
		SetDuplicateDetection(window),
	)
}