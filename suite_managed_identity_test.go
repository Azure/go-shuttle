// +build !withconnectionstring

package pubsub

import (
	"time"
)

// TestPublishAndListenWithManagedIdentityUsingDefault tests both the publisher and listener with default configurations
func (suite *serviceBusSuite) TestPublishAndListenWithManagedIdentityUsingDefault() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithManagedIdentity(suite.TopicName)
	suite.NoError(err)
	listener, err := createNewListenerWithManagedIdentity()
	suite.NoError(err)

	suite.defaultTest(publisher, listener)
}

// TestPublishAndListenWithManagedIdentityUsingDefault tests both the publisher and listener with default configurations
func (suite *serviceBusSuite) TestPublishAndListenWithManagedIdentityResourceIDUsingDefault() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithManagedIdentityResourceID(suite.TopicName)
	suite.NoError(err)
	listener, err := createNewListenerWithManagedIdentityResourceID()
	suite.NoError(err)

	suite.defaultTest(publisher, listener)
}

// TestPublishAndListenWithManagedIdentityUsingTypeFilter tests both the publisher and listener with a filter on the event type
func (suite *serviceBusSuite) TestPublishAndListenWithManagedIdentityUsingTypeFilter() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithManagedIdentity(suite.TopicName)
	suite.NoError(err)
	listener, err := createNewListenerWithManagedIdentity()
	suite.NoError(err)

	suite.typeFilterTest(publisher, listener)
}

// TestPublishAndListenWithManagedIdentityUsingTypeFilter tests both the publisher and listener with a filter on the event type
func (suite *serviceBusSuite) TestPublishAndListenWithManagedIdentityResourceIDUsingTypeFilter() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithManagedIdentityResourceID(suite.TopicName)
	suite.NoError(err)
	listener, err := createNewListenerWithManagedIdentityResourceID()
	suite.NoError(err)

	suite.typeFilterTest(publisher, listener)
}

// TestPublishAndListenWithManagedIdentityUsingCustomHeaderFilter tests both the publisher and listener with a customer filter
func (suite *serviceBusSuite) TestPublishAndListenWithManagedIdentityUsingCustomHeaderFilter() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithManagedIdentityUsingCustomHeader(suite.TopicName, "testHeader", "Key")
	suite.NoError(err)
	listener, err := createNewListenerWithManagedIdentityResourceID()
	suite.NoError(err)

	suite.customHeaderFilterTest(publisher, listener)
}

// TestPublishAndListenWithConnectionStringUsingDuplicateDetection tests both the publisher and listener with duplicate detection
func (suite *serviceBusSuite) TestPublishAndListenWithManagedIdentityUsingDuplicateDetection() {
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	dupeDetectionTopicName := testTopicName + "dupedetection-managedidentity" + suite.TagID
	dupeDetectionWindow := 5 * time.Minute
	publisher, err := createNewPublisherWithManagedIdentityUsingDuplicateDetection(dupeDetectionTopicName, &dupeDetectionWindow)
	suite.NoError(err)
	listener, err := createNewListenerWithManagedIdentityResourceID()
	suite.NoError(err)

	suite.duplicateDetectionTest(publisher, listener, dupeDetectionTopicName)
}

func (suite *serviceBusSuite) TestPublishAndListenRetryLaterWithManagedIdentity() {
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	retryLaterTopic := testTopicName + "retrylater-managedidentity" + suite.TagID
	publisher, err := createNewPublisherWithManagedIdentity(retryLaterTopic)
	suite.NoError(err)
	listener, err := createNewListenerWithConnectionString()
	suite.NoError(err)
	// create retryLater event. listener emits retry based on event type
	event := &retryLaterEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	suite.publishAndReceiveMessageWithRetryAfter(publishReceiveTest{
		topicName:       retryLaterTopic,
		listener:        listener,
		publisher:       publisher,
		listenerOptions: []ListenerOption{SetSubscriptionName("retrylatersub" + suite.TagID)},
		shouldSucceed:   true,
	}, event)
}
