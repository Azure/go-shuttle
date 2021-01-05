package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/internal/reflection"
	"github.com/Azure/go-shuttle/listener"
	"github.com/Azure/go-shuttle/message"
	"github.com/Azure/go-shuttle/publisher"
	"github.com/stretchr/testify/assert"
)

// TestPublishAndListenWithConnectionStringUsingDefault tests both the publisher and listener with default configurations
func (suite *serviceBusSuite) TestPublishAndListenUsingDefault() {
	pub, err := publisher.New(suite.TopicName, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := listener.New(suite.listenerAuthOption, listener.WithSubscriptionName("defaultTestSub"))
	suite.NoError(err)

	suite.defaultTest(pub, l)
}

// TestPublishAndListenMessageTwice tests publish and listen the same messages twice
func (suite *serviceBusSuite) TestPublishAndListenMessageTwice() {
	pub, err := publisher.New(suite.TopicName, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := listener.New(suite.listenerAuthOption, listener.WithSubscriptionName("testTwoMessages"))
	suite.NoError(err)

	suite.defaultTestWithMessageTwice(pub, l)
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

// Note that, this test need manually verify, you should be able to see 3 msgs are receievd in a row from the test log
// if the listener concurrency is working well.
func (suite *serviceBusSuite) TestPublishAndConcurrentListen() {
	suite.T().Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	concurrentListenTopic := suite.Prefix + "concurrentlisten" + suite.TagID
	pub, err := publisher.New(concurrentListenTopic, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := listener.New(
		suite.listenerAuthOption,
		listener.WithSubscriptionName("concurrentListen"))
	suite.NoError(err)

	numOfMsgs := 6
	suite.publishAndConcurrentReceiveMessage(publishReceiveTest{
		topicName:       concurrentListenTopic,
		listener:        l,
		publisher:       pub,
		listenerOptions: []listener.Option{},
		shouldSucceed:   true,
	}, numOfMsgs)
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

func (suite *serviceBusSuite) defaultTestWithMessageTwice(p *publisher.Publisher, l *listener.Listener) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key1",
		Value: "value1",
	}
	suite.publishAndReceiveMessageTwice(
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

func (suite *serviceBusSuite) publishAndConcurrentReceiveMessage(testConfig publishReceiveTest, numOfEvents int) {
	ctx := context.Background()
	if testConfig.listenerOptions == nil {
		testConfig.listenerOptions = []listener.Option{}
	}
	if testConfig.publisherOptions == nil {
		testConfig.publisherOptions = []publisher.Option{}
	}

	testConfig.listenerOptions = append(testConfig.listenerOptions, listener.WithConcurrency(3))

	// setup listener
	gotMessage := make(chan string)
	var order int32
	go func() {
		err := testConfig.listener.Listen(
			ctx,
			checkConcurrentResultHandler(gotMessage, &order),
			testConfig.topicName,
			testConfig.listenerOptions...,
		)
		if err != nil {
			fmt.Printf("listener error: %s", err)
		}
	}()
	// publish after the listener is setup
	time.Sleep(5 * time.Second)
	for i := 0; i < numOfEvents; i++ {
		err := testConfig.publisher.Publish(
			ctx,
			testEvent{ID: i + 1},
			testConfig.publisherOptions...,
		)
		suite.NoError(err)
	}
	var msgstatus []string
	expected := []string{
		"recieved1",
		"recieved2",
		"recieved3",
		"completed1",
		"recieved4",
		"completed2",
		"recieved5",
		"completed3",
		"recieved6",
		"completed4",
		"completed5",
		"completed6",
	}

	for {
		select {
		case status := <-gotMessage:
			if status == "error" && testConfig.shouldSucceed {
				suite.FailNow("unhappy response")
			}
			msgstatus = append(msgstatus, status)

		case <-time.After(10 * time.Second):
			if testConfig.shouldSucceed {
				suite.FailNow("Test didn't finish on time %v", msgstatus)
			}
		}
		if len(msgstatus) >= len(expected) {
			break
		}
	}

	//first three can be any order but after that should be same order.
	assert.Subset(suite.T(), expected, msgstatus)
	assert.Equal(suite.T(), msgstatus[3:], expected[3:])

	//time.Sleep(20 * time.Second)
	err := testConfig.listener.Close(ctx)
	suite.NoError(err)
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

func (suite *serviceBusSuite) publishAndReceiveMessageTwice(testConfig publishReceiveTest, event interface{}) {
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

	// publish same message again
	time.Sleep(5 * time.Second)
	err = testConfig.publisher.Publish(
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

func checkConcurrentResultHandler(ch chan<- string, order *int32) message.Handler {
	return message.HandleFunc(
		func(ctx context.Context, msg *message.Message) message.Handler {
			order := atomic.AddInt32(order, 1)
			ch <- fmt.Sprintf("recieved%d", order)
			//fmt.Printf("concurrent Got a new Msg : %d, %v, %s\n", order, time.Now(), msg.Data())
			time.Sleep(time.Duration(order) * time.Second)
			ch <- fmt.Sprintf("completed%d", order)
			//fmt.Printf("concurrent completed : %d, %v, %s\n", order, time.Now(), msg.Data())
			return message.Complete()
		})
}
