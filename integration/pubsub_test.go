package integration

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/internal/reflection"
	"github.com/Azure/go-shuttle/listener"
	"github.com/Azure/go-shuttle/message"
	"github.com/Azure/go-shuttle/publisher"
)

// TestPublishAndListenWithConnectionStringUsingDefault tests both the publisher and listener with default configurations
func (suite *serviceBusSuite) TestPublishAndListenUsingDefault() {
	pub, err := publisher.New(suite.TopicName, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := listener.New(suite.listenerAuthOption, listener.WithSubscriptionName("defaultTestSub"))
	suite.NoError(err)

	suite.defaultTest(pub, l)
}

// TestPublishAndListenWithConnectionStringUsingTypeFilter tests both the publisher and listener with a filter on the event type
func (suite *serviceBusSuite) TestPublishAndListenUsingTypeFilter() {
	pub, err := publisher.New(suite.TopicName, suite.publisherAuthOption)
	suite.NoError(err)
	successListener, err := listener.New(suite.listenerAuthOption,
		listener.WithSubscriptionName("subTypeFilter"),
		listener.WithFilterDescriber("testFilter", servicebus.SQLFilter{Expression: "type LIKE 'testEvent'"}))
	suite.NoError(err)
	failListener, err := listener.New(suite.listenerAuthOption,
		listener.WithSubscriptionName("subTypeFilter"),
		listener.WithFilterDescriber("testFilter", servicebus.SQLFilter{Expression: "type LIKE 'nottestEvent'"}))

	suite.typeFilterTest(pub, successListener, true)
	suite.typeFilterTest(pub, failListener, false)
}

// TestPublishAndListenUsingCustomHeaderFilter tests both the publisher and listener with a customer filter
func (suite *serviceBusSuite) TestPublishAndListenUsingCustomHeaderFilter() {
	suite.T().Parallel()
	// this assumes that the testTopic was created at the start of the test suite
	pub, err := publisher.New(
		suite.TopicName,
		suite.publisherAuthOption,
		publisher.SetDefaultHeader("testHeader", "Key"))
	suite.NoError(err)
	successListener, err := listener.New(suite.listenerAuthOption,
		listener.WithSubscriptionName("subNameHeader"),
		listener.WithFilterDescriber("testFilter", servicebus.SQLFilter{Expression: "testHeader LIKE 'key'"}))
	suite.NoError(err)
	failListener, err := listener.New(suite.listenerAuthOption,
		listener.WithSubscriptionName("subNameHeader"),
		listener.WithFilterDescriber("testFilter", servicebus.SQLFilter{Expression: "testHeader LIKE 'notkey'"}))
	suite.NoError(err)

	suite.customHeaderFilterTest(pub, successListener, true)
	suite.customHeaderFilterTest(pub, failListener, false)
}

// TestPublishAndListenWithConnectionStringUsingDuplicateDetection tests both the publisher and listener with duplicate detection
func (suite *serviceBusSuite) TestPublishAndListenUsingDuplicateDetection() {
	suite.T().Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	dupeDetectionTopicName := suite.Prefix + "deduptopic" + suite.TagID
	dupeDetectionWindow := 5 * time.Minute
	pub, err := publisher.New(
		dupeDetectionTopicName,
		suite.publisherAuthOption,
		publisher.WithDuplicateDetection(&dupeDetectionWindow))
	suite.NoError(err)
	l, err := listener.New(suite.listenerAuthOption, listener.WithSubscriptionName("subDedup"))
	suite.NoError(err)
	suite.duplicateDetectionTest(pub, l, dupeDetectionTopicName)
}

func (suite *serviceBusSuite) TestPublishAndListenRetryLater() {
	suite.T().Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	retryLaterTopic := suite.Prefix + "retrylater" + suite.TagID
	pub, err := publisher.New(retryLaterTopic, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := listener.New(
		suite.listenerAuthOption,
		listener.WithSubscriptionName("subRetryLater"))
	suite.NoError(err)
	// create retryLater event. listener emits retry based on event type
	event := &retryLaterEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	suite.publishAndReceiveMessageWithRetryAfter(publishReceiveTest{
		topicName:       retryLaterTopic,
		listener:        l,
		publisher:       pub,
		listenerOptions: []listener.Option{},
		shouldSucceed:   true,
	}, event)
}

func (suite *serviceBusSuite) defaultTest(p *publisher.Publisher, l *listener.Listener) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			topicName:     suite.TopicName,
			listener:      l,
			publisher:     p,
			shouldSucceed: true,
		},
		event,
	)
}

func (suite *serviceBusSuite) typeFilterTest(p *publisher.Publisher, l *listener.Listener, shouldSucceed bool) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with a filter on the event type
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			topicName:     suite.TopicName,
			listener:      l,
			publisher:     p,
			shouldSucceed: shouldSucceed,
		},
		event,
	)
}

func (suite *serviceBusSuite) customHeaderFilterTest(pub *publisher.Publisher, l *listener.Listener, shouldSucceed bool) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with a filter on the custom header
	suite.publishAndReceiveMessage(
		publishReceiveTest{
			topicName:     suite.TopicName,
			listener:      l,
			publisher:     pub,
			shouldSucceed: shouldSucceed,
		},
		event,
	)
}

func (suite *serviceBusSuite) duplicateDetectionTest(pub *publisher.Publisher, l *listener.Listener, topicName string) {
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
			listener:         l,
			publisher:        pub,
			publisherOptions: []publisher.Option{publisher.SetMessageID("hi")},
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
			topicName:     topicName,
			listener:      l,
			publisher:     pub,
			shouldSucceed: true,
		},
		event2,
	)
}

func (suite *serviceBusSuite) publishAndReceiveMessage(testConfig publishReceiveTest, event interface{}) {
	ctx := context.Background()
	gotMessage := make(chan bool)
	if testConfig.listenerOptions == nil {
		testConfig.listenerOptions = []listener.Option{}
	}
	if testConfig.publisherOptions == nil {
		testConfig.publisherOptions = []publisher.Option{}
	}

	// setup listener
	go func() {
		eventJSON, err := json.Marshal(event)
		suite.NoError(err)
		err = testConfig.listener.Listen(
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

func (suite *serviceBusSuite) publishAndReceiveMessageWithRetryAfter(testConfig publishReceiveTest, event interface{}) {
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
	case <-time.After(5 * time.Second):
		if testConfig.shouldSucceed {
			suite.FailNow("Test didn't finish on time")
		}
	}
	err := testConfig.listener.Close(ctx)
	suite.NoError(err)
}

func checkResultHandler(publishedMsg string, publishedMsgType string, ch chan<- bool) message.Handler {
	var retryCount = 0
	return message.HandleFunc(
		func(ctx context.Context, msg *message.Message) message.Handler {
			if publishedMsg != msg.Data() {
				ch <- false
				return message.Error(errors.New("published message and received message are different"))
			}
			if publishedMsgType != msg.Type() {
				ch <- false
				return message.Error(errors.New("published message type and received message type are different"))
			}
			if publishedMsgType == reflection.GetType(retryLaterEvent{}) {
				if retryCount > 0 {
					ch <- true
					return message.Complete()
				}
				retryCount++
				return message.RetryLater(1 * time.Second)
			}
			ch <- true
			return message.Complete()
		})
}
