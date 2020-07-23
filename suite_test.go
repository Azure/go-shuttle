package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/keikumata/azure-pub-sub/internal/test"
	"github.com/stretchr/testify/suite"
)

type serviceBusSuite struct {
	test.BaseSuite
	TopicName string
}

type testEvent struct {
	ID    int    `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

const (
	defaultTimeout = 60 * time.Second
	testTopicName  = "testTopic"
)

func TestPubSub(t *testing.T) {
	suite.Run(t, new(serviceBusSuite))
}

func (suite *serviceBusSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
	suite.TopicName = testTopicName + suite.TagID
	_, err := suite.EnsureTopic(context.Background(), suite.TopicName)
	if err != nil {
		suite.T().Fatal(err)
	}
}

type publishReceiveTest struct {
	listener         *Listener
	publisher        *Publisher
	filter           servicebus.FilterDescriber
	listenerOptions  []ListenerOption
	publisherOptions []PublisherOption
	shouldSucceed    bool
}

// TestPublishAndListenWithDefault tests both the publisher and listener with default configurations
func (suite *serviceBusSuite) TestPublishAndListenWithDefault() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherFromConnectionString(suite.TopicName)
	suite.NoError(err)
	listener, err := createNewListenerFromConnectionString()
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

// TestPublishAndListenWithTypeFilter tests both the publisher and listener with a filter on the event type
func (suite *serviceBusSuite) TestPublishAndListenWithTypeFilter() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherFromConnectionString(suite.TopicName)
	suite.NoError(err)
	listener, err := createNewListenerFromConnectionString()
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

// TestPublishAndListenWithCustomHeaderFilter tests both the publisher and listener with a customer filter
func (suite *serviceBusSuite) TestPublishAndListenWithCustomHeaderFilter() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithCustomHeader(suite.TopicName, "testHeader", "Key")
	suite.NoError(err)
	listener, err := createNewListenerFromConnectionString()
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

// TestPublishAndListenWithCustomHeaderFilter tests both the publisher and listener with a customer filter
func (suite *serviceBusSuite) TestPublishAndListenWithDuplicateDetection() {
	// this assumes that the testTopic was created at the start of the test suite
	dupeDetectionWindow := 5 * time.Minute
	publisher, err := createNewPublisherWithDuplicateDetection(suite.TopicName, &dupeDetectionWindow)
	suite.NoError(err)
	listener, err := createNewListenerFromConnectionString()
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
			listener:         listener,
			publisher:        publisher,
			listenerOptions:  []ListenerOption{SetSubscriptionName("subName3")},
			publisherOptions: []PublisherOption{SetMessageID("hi")},
			filter:           &servicebus.SQLFilter{Expression: "testHeader LIKE 'key'"},
			shouldSucceed:    true,
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

func (suite *serviceBusSuite) publishAndReceiveMessage(testConfig publishReceiveTest, event interface{}) {
	ctx := context.Background()
	gotMessage := make(chan bool)

	// setup listener
	go func() {
		eventJSON, err := json.Marshal(event)
		suite.NoError(err)
		var listenerOpts []ListenerOption
		if testConfig.filter == nil {
			listenerOpts = testConfig.listenerOptions
		} else {
			listenerOpts = append(
				testConfig.listenerOptions,
				SetSubscriptionFilter("testFilter", testConfig.filter),
			)
		}
		testConfig.listener.Listen(
			ctx,
			checkResultHandler(string(eventJSON), gotMessage),
			suite.TopicName,
			listenerOpts...,
		)
	}()
	// publish after the listener is setup
	time.Sleep(5 * time.Second)
	err := testConfig.publisher.Publish(
		ctx,
		event,
		testConfig.publisherOptions...,
	)
	suite.NoError(err)

	select {
	case isSuccessful := <-gotMessage:
		if testConfig.shouldSucceed != isSuccessful {
			suite.FailNow("Test did not succeed")
		}
	case <-time.After(10 * time.Second):
		if testConfig.shouldSucceed {
			suite.FailNow("Test didn't finish on time")
		}
	}
	err = testConfig.listener.Close(ctx)
	suite.NoError(err)
}

func checkResultHandler(publishedMsg string, ch chan<- bool) Handle {
	return func(ctx context.Context, message string, messageType string) error {
		if publishedMsg != message {
			ch <- false
			return errors.New("published message and received message are different")
		}
		ch <- true
		return nil
	}
}
