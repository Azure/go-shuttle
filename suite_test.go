package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/keikumata/azure-pub-sub/internal/reflection"
	"github.com/keikumata/azure-pub-sub/internal/test"
	"github.com/stretchr/testify/suite"
)

type serviceBusSuite struct {
	test.BaseSuite
	TopicName string
	Publisher Publisher
	Listener  Listener
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
	topicName        string
	listener         *Listener
	publisher        *Publisher
	listenerOptions  []ListenerOption
	publisherOptions []PublishOption
	publishCount     *int
	shouldSucceed    bool
}

// TestPublishAndListenWithConnectionStringUsingDefault tests both the publisher and listener with default configurations
func (suite *serviceBusSuite) TestPublishAndListenWithConnectionStringUsingDefault() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithConnectionString(suite.TopicName)
	suite.NoError(err)
	listener, err := createNewListenerWithConnectionString()
	suite.NoError(err)

	suite.defaultTest(publisher, listener)
}

// TestPublishAndListenWithConnectionStringUsingTypeFilter tests both the publisher and listener with a filter on the event type
func (suite *serviceBusSuite) TestPublishAndListenWithConnectionStringUsingTypeFilter() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithConnectionString(suite.TopicName)
	suite.NoError(err)
	listener, err := createNewListenerWithConnectionString()
	suite.NoError(err)

	suite.typeFilterTest(publisher, listener)
}

// TestPublishAndListenWithConnectionStringUsingCustomHeaderFilter tests both the publisher and listener with a customer filter
func (suite *serviceBusSuite) TestPublishAndListenWithConnectionStringUsingCustomHeaderFilter() {
	// this assumes that the testTopic was created at the start of the test suite
	publisher, err := createNewPublisherWithConnectionStringUsingCustomHeader(suite.TopicName, "testHeader", "Key")
	suite.NoError(err)
	listener, err := createNewListenerWithConnectionString()
	suite.NoError(err)

	suite.customHeaderFilterTest(publisher, listener)
}

// TestPublishAndListenWithConnectionStringUsingDuplicateDetection tests both the publisher and listener with duplicate detection
func (suite *serviceBusSuite) TestPublishAndListenWithConnectionStringUsingDuplicateDetection() {
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	dupeDetectionTopicName := testTopicName + "dupedetection" + suite.TagID
	dupeDetectionWindow := 5 * time.Minute
	publisher, err := createNewPublisherWithConnectionStringUsingDuplicateDetection(dupeDetectionTopicName, &dupeDetectionWindow)
	suite.NoError(err)
	listener, err := createNewListenerWithConnectionString()
	suite.NoError(err)

	suite.duplicateDetectionTest(publisher, listener, dupeDetectionTopicName)
}

func (suite *serviceBusSuite) defaultTest(publisher *Publisher, listener *Listener) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			topicName:       suite.TopicName,
			listener:        listener,
			publisher:       publisher,
			listenerOptions: []ListenerOption{SetSubscriptionName("subName1")},
			shouldSucceed:   true,
		},
		event,
	)
}

func (suite *serviceBusSuite) typeFilterTest(publisher *Publisher, listener *Listener) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with a filter on the event type
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			topicName: suite.TopicName,
			listener:  listener,
			publisher: publisher,
			listenerOptions: []ListenerOption{
				SetSubscriptionName("subName2"),
				SetSubscriptionFilter("testFilter", servicebus.SQLFilter{Expression: "type LIKE 'testEvent'"}),
			},
			shouldSucceed: true,
		},
		event,
	)
	// test with a filter on the the wrong type - should fail
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			topicName: suite.TopicName,
			listener:  listener,
			publisher: publisher,
			listenerOptions: []ListenerOption{
				SetSubscriptionName("subName2"),
				SetSubscriptionFilter("testFilter", servicebus.SQLFilter{Expression: "type LIKE 'testEvent2'"}),
			},
			shouldSucceed: false,
		},
		event,
	)
}

func (suite *serviceBusSuite) customHeaderFilterTest(publisher *Publisher, listener *Listener) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with a filter on the custom header
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			topicName: suite.TopicName,
			listener:  listener,
			publisher: publisher,
			listenerOptions: []ListenerOption{
				SetSubscriptionName("subName3"),
				SetSubscriptionFilter("testFilter", servicebus.SQLFilter{Expression: "testHeader LIKE 'key'"}),
			},
			shouldSucceed: true,
		},
		event,
	)
	// test with a filter on the custom header but wrong value
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			topicName: suite.TopicName,
			listener:  listener,
			publisher: publisher,
			listenerOptions: []ListenerOption{
				SetSubscriptionName("subName3"),
				SetSubscriptionFilter("testFilter", servicebus.SQLFilter{Expression: "testHeader LIKE 'notkey'"}),
			},
			shouldSucceed: false,
		},
		event,
	)
}

func (suite *serviceBusSuite) duplicateDetectionTest(publisher *Publisher, listener *Listener, topicName string) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with duplicate detection
	publishCount := 2
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			topicName:        topicName,
			listener:         listener,
			publisher:        publisher,
			listenerOptions:  []ListenerOption{SetSubscriptionName("subName4")},
			publisherOptions: []PublishOption{SetMessageID("hi")},
			publishCount:     &publishCount,
			shouldSucceed:    true,
		},
		event,
	)

	// create another test event. if dupe detection didn't work then this test will fail because the listener will
	// receive the first event and not the second event
	event2 := &testEvent{
		ID:    2,
		Key:   "key2",
		Value: "value2",
	}
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			topicName:       topicName,
			listener:        listener,
			publisher:       publisher,
			listenerOptions: []ListenerOption{SetSubscriptionName("subName4")},
			shouldSucceed:   true,
		},
		event2,
	)
}

func (suite *serviceBusSuite) publishAndReceiveMessage(testConfig publishReceiveTest, event interface{}) {
	ctx := context.Background()
	gotMessage := make(chan bool)

	// setup listener
	go func() {
		eventJSON, err := json.Marshal(event)
		suite.NoError(err)
		testConfig.listener.Listen(
			ctx,
			checkResultHandler(string(eventJSON), reflection.GetType(event), gotMessage),
			testConfig.topicName,
			testConfig.listenerOptions...,
		)
	}()
	// publish after the listener is setup
	time.Sleep(5 * time.Second)
	publishCount := 1
	if testConfig.publishCount != nil {
		publishCount = *testConfig.publishCount
	}
	for i := 0; i < publishCount; i++ {
		err := testConfig.publisher.Publish(
			ctx,
			event,
			testConfig.publisherOptions...,
		)
		suite.NoError(err)
	}

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
	err := testConfig.listener.Close(ctx)
	suite.NoError(err)
}

func checkResultHandler(publishedMsg string, publishedMsgType string, ch chan<- bool) Handle {
	return func(ctx context.Context, message string, messageType string) error {
		if publishedMsg != message {
			ch <- false
			return errors.New("published message and received message are different")
		}
		if publishedMsgType != messageType {
			ch <- false
			return errors.New("published message type and received message type are different")
		}
		ch <- true
		return nil
	}
}
