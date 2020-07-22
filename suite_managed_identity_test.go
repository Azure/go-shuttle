// +build !withconnectionstring

package pubsub

import (
	servicebus "github.com/Azure/azure-service-bus-go"
)

// TestPublishAndListenWithManagedIdentityUsingDefault tests both the publisher and listener with default configurations
func (suite *serviceBusSuite) TestPublishAndListenWithManagedIdentityUsingDefault() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithManagedIdentity(suite.TopicName)
	suite.NoError(err)
	listener, err := createNewListenerWithManagedIdentity()
	suite.NoError(err)

	// create test event
	testEvent := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			listener:        listener,
			publisher:       publisher,
			listenerOptions: []ListenerOption{SetSubscriptionName("subName1")},
			shouldSucceed:   true,
		},
		testEvent,
	)
}

// TestPublishAndListenWithManagedIdentityUsingTypeFilter tests both the publisher and listener with a filter on the event type
func (suite *serviceBusSuite) TestPublishAndListenWithManagedIdentityUsingTypeFilter() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithManagedIdentity(suite.TopicName)
	suite.NoError(err)
	listener, err := createNewListenerWithManagedIdentity()
	suite.NoError(err)

	// create test event
	testEvent := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with a filter on the event type
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			listener:        listener,
			publisher:       publisher,
			listenerOptions: []ListenerOption{SetSubscriptionName("subName2")},
			filter:          servicebus.SQLFilter{Expression: "type LIKE 'testEvent'"},
			shouldSucceed:   true,
		},
		testEvent,
	)
	// test with a filter on the the wrong type - should fail
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			listener:        listener,
			publisher:       publisher,
			listenerOptions: []ListenerOption{SetSubscriptionName("subName2")},
			filter:          &servicebus.SQLFilter{Expression: "type LIKE 'testEvent2'"},
			shouldSucceed:   false,
		},
		testEvent,
	)
}


// TestPublishAndListenWithManagedIdentityUsingCustomHeaderFilter tests both the publisher and listener with a customer filter
func (suite *serviceBusSuite) TestPublishAndListenWithManagedIdentityUsingCustomHeaderFilter() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithManagedIdentityUsingCustomHeader(suite.TopicName, "testHeader", "Key")
	suite.NoError(err)
	listener, err := createNewListenerWithManagedIdentity()
	suite.NoError(err)

	// create test event
	testEvent := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with a filter on the custom header
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			listener:        listener,
			publisher:       publisher,
			listenerOptions: []ListenerOption{SetSubscriptionName("subName3")},
			filter:          &servicebus.SQLFilter{Expression: "testHeader LIKE 'key'"},
			shouldSucceed:   true,
		},
		testEvent,
	)
	// test with a filter on the custom header but wrong value
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			listener:        listener,
			publisher:       publisher,
			listenerOptions: []ListenerOption{SetSubscriptionName("subName3")},
			filter:          servicebus.SQLFilter{Expression: "testHeader LIKE 'notkey'"},
			shouldSucceed:   false,
		},
		testEvent,
	)
}