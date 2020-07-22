package pubsub

import (
	"context"
	"errors"
	"os"
	"time"
)

// TestCreatePublisherFromConnectionStringWithNewTopic tests the creation of a publisher for a new topic and a connection string
func (suite *serviceBusSuite) TestCreatePublisherWithConnectionStringUsingNewTopic() {
	topicName := "newTopic" + suite.TagID
	publisher, err := createNewPublisherWithConnectionString(topicName)
	if suite.NoError(err) {
		connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING")
		suite.Contains(connStr, publisher.namespace.Name)

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
func (suite *serviceBusSuite) TestCreatePublisherWithConnectionStringUsingExistingTopic() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithConnectionString(suite.TopicName)
	if suite.NoError(err) {
		connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING")
		suite.Contains(connStr, publisher.namespace.Name)

		// make sure that topic exists
		ns := suite.GetNewNamespace()
		tm := ns.NewTopicManager()
		_, err := tm.Get(context.Background(), suite.TopicName)
		suite.NoError(err)
	}
}

func createNewPublisherWithConnectionString(topicName string) (*Publisher, error) {
	connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if connStr == "" {
		return nil, errors.New("environment variable SERVICEBUS_CONNECTION_STRING was not set")
	}

	return NewPublisher(topicName, PublisherWithConnectionString(connStr))
}

func createNewPublisherWithConnectionStringUsingCustomHeader(topicName, headerName, msgKey string) (*Publisher, error) {
	connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if connStr == "" {
		return nil, errors.New("environment variable SERVICEBUS_CONNECTION_STRING was not set")
	}

	return NewPublisher(
		topicName,
		PublisherWithConnectionString(connStr),
		SetDefaultHeader(headerName, msgKey),
	)
}

func createNewPublisherWithDuplicateDetection(topicName string, window *time.Duration) (*Publisher, error) {
	connStr := os.Getenv("SERVICEBUS_CONNECTION_STRING") // `Endpoint=sb://XXXX.servicebus.windows.net/;SharedAccessKeyName=XXXX;SharedAccessKey=XXXX`
	if connStr == "" {
		return nil, errors.New("environment variable SERVICEBUS_CONNECTION_STRING was not set")
	}

	return NewPublisher(
		topicName,
		PublisherWithConnectionString(connStr),
		SetDuplicateDetection(window),
	)
}
