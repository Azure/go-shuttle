// +build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Azure/go-shuttle/publisher/topic"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/internal/reflection"
	"github.com/Azure/go-shuttle/listener"
	"github.com/Azure/go-shuttle/message"
	"github.com/devigned/tab"
	"github.com/stretchr/testify/assert"
)

// TestPublishAndListenWithConnectionStringUsingDefault tests both the publisher and listener with default configurations
func (suite *serviceBusTopicSuite) TestPublishAndListenUsingDefault() {
	pub, err := topic.New(context.Background(), suite.TopicName, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := listener.New(suite.listenerAuthOption, listener.WithSubscriptionName("defaultTestSub"))
	suite.NoError(err)

	suite.defaultTest(pub, l)
}

// TestPublishAndListenMessageTwice tests publish and listen the same messages twice
func (suite *serviceBusTopicSuite) TestPublishAndListenMessageTwice() {
	pub, err := topic.New(context.Background(), suite.TopicName, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := listener.New(suite.listenerAuthOption, listener.WithSubscriptionName("testTwoMessages"))
	suite.NoError(err)

	suite.defaultTestWithMessageTwice(pub, l)
}

// TestPublishAndListenWithConnectionStringUsingTypeFilter tests both the publisher and listener with a filter on the event type
func (suite *serviceBusTopicSuite) TestPublishAndListenUsingFilter() {
	pub, err := topic.New(context.Background(), suite.TopicName, suite.publisherAuthOption)
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

type notTestEvent struct {
	wrongType string
}

// TestPublishAndListenWithConnectionStringUsingTypeFilter tests both the publisher and listener with a filter on the event type
func (suite *serviceBusTopicSuite) TestPublishAndListenUsingTypeFilter() {
	pub, err := topic.New(context.Background(), suite.TopicName, suite.publisherAuthOption)
	suite.NoError(err)
	// listener with wrong event type. not getting the event
	failListener, err := listener.New(suite.listenerAuthOption,
		listener.WithSubscriptionName("subEventTypeFilterFail"),
		listener.WithTypeFilter(&notTestEvent{}))
	suite.NoError(err)
	suite.typeFilterTest(pub, failListener, false)

	// update subscription to filter on correct event type. succeeds receiving event
	successListener, err := listener.New(suite.listenerAuthOption,
		listener.WithSubscriptionName("subEventTypeFilter"),
		listener.WithTypeFilter(&testEvent{}))
	suite.typeFilterTest(pub, successListener, true)
}

// TestPublishAndListenUsingCustomHeaderFilter tests both the publisher and listener with a customer filter
func (suite *serviceBusTopicSuite) TestPublishAndListenUsingCustomHeaderFilter() {
	suite.T().Parallel()
	// this assumes that the testTopic was created at the start of the test suite
	pub, err := topic.New(
		context.Background(),
		suite.TopicName,
		suite.publisherAuthOption,
		topic.SetDefaultHeader("testHeader", "Key"))
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
func (suite *serviceBusTopicSuite) TestPublishAndListenUsingDuplicateDetection() {
	suite.T().Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	dupeDetectionTopicName := suite.Prefix + "deduptopic" + suite.TagID
	dupeDetectionWindow := 5 * time.Minute
	pub, err := topic.New(
		context.Background(),
		dupeDetectionTopicName,
		suite.publisherAuthOption,
		topic.WithDuplicateDetection(&dupeDetectionWindow))
	suite.NoError(err)
	l, err := listener.New(suite.listenerAuthOption, listener.WithSubscriptionName("subDedup"))
	suite.NoError(err)
	suite.duplicateDetectionTest(pub, l, dupeDetectionTopicName)
}

func (suite *serviceBusTopicSuite) TestPublishAndListenRetryLater() {
	suite.T().Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	retryLaterTopic := suite.Prefix + "retrylater" + suite.TagID
	pub, err := topic.New(context.Background(), retryLaterTopic, suite.publisherAuthOption)
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

func (suite *serviceBusTopicSuite) TestPublishAndListenShortLockDuration() {
	suite.T().Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	shortLockTopic := suite.Prefix + "shortlock" + suite.TagID
	pub, err := topic.New(context.Background(), shortLockTopic, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := listener.New(
		suite.listenerAuthOption,
		listener.WithSubscriptionDetails(2*time.Second, 3),
		listener.WithSubscriptionName("subshortlock"))
	suite.NoError(err)
	// create retryLater event. listener emits retry based on event type
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	suite.publishAndReceiveMessageWithAutoLockRenewal(publishReceiveTest{
		topicName:       shortLockTopic,
		listener:        l,
		publisher:       pub,
		listenerOptions: []listener.Option{listener.WithMessageLockAutoRenewal(1 * time.Second)},
		shouldSucceed:   true,
	}, event)
}

func (suite *serviceBusTopicSuite) TestPublishAndListenNotRenewingLock() {
	suite.T().Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	norenewlockTopic := suite.Prefix + "norenewlock" + suite.TagID
	pub, err := topic.New(context.Background(), norenewlockTopic, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := listener.New(
		suite.listenerAuthOption,
		listener.WithSubscriptionDetails(2*time.Second, 2),
		listener.WithSubscriptionName("norenewlock"))
	suite.NoError(err)
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	suite.publishAndReceiveMessageNotRenewingLock(publishReceiveTest{
		topicName:       norenewlockTopic,
		listener:        l,
		publisher:       pub,
		listenerOptions: []listener.Option{},
		shouldSucceed:   false,
	}, event)
}

func (suite *serviceBusTopicSuite) TestPublishAndListenConcurrentPrefetch() {
	suite.T().Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	prefetchTopic := suite.Prefix + "prefetch" + suite.TagID
	pub, err := topic.New(context.Background(), prefetchTopic, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := listener.New(
		suite.listenerAuthOption,
		listener.WithSubscriptionDetails(60*time.Second, 2),
		listener.WithSubscriptionName("prefetch"))
	suite.NoError(err)
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	publishCount := 200
	suite.publishAndReceiveMessageWithPrefetch(publishReceiveTest{
		topicName: prefetchTopic,
		listener:  l,
		publisher: pub,
		listenerOptions: []listener.Option{
			listener.WithPrefetchCount(50),
			listener.WithMessageLockAutoRenewal(10 * time.Second),
			listener.WithMaxConcurrency(50),
		},
		shouldSucceed: true,
		publishCount:  &publishCount,
	}, event)
}

func (suite *serviceBusTopicSuite) defaultTest(p *topic.Publisher, l *listener.Listener) {
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

func (suite *serviceBusTopicSuite) defaultTestWithMessageTwice(p *topic.Publisher, l *listener.Listener) {
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

func (suite *serviceBusTopicSuite) typeFilterTest(p *topic.Publisher, l *listener.Listener, shouldSucceed bool) {
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

func (suite *serviceBusTopicSuite) customHeaderFilterTest(pub *topic.Publisher, l *listener.Listener, shouldSucceed bool) {
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

func (suite *serviceBusTopicSuite) duplicateDetectionTest(pub *topic.Publisher, l *listener.Listener, topicName string) {
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
			publisherOptions: []topic.Option{topic.SetMessageID("hi")},
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

func (suite *serviceBusTopicSuite) publishAndReceiveMessage(testConfig publishReceiveTest, event interface{}) {
	ctx := context.Background()
	gotMessage := make(chan bool)
	if testConfig.listenerOptions == nil {
		testConfig.listenerOptions = []listener.Option{}
	}
	if testConfig.publisherOptions == nil {
		testConfig.publisherOptions = []topic.Option{}
	}

	// setup listener
	go func() {
		eventJSON, err := json.Marshal(event)
		suite.NoError(err)
		err = testConfig.listener.Listen(
			ctx,
			checkResultHandler(string(eventJSON), reflection.GetType(testEvent{}), gotMessage),
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
	case <-time.After(15 * time.Second):
		if testConfig.shouldSucceed {
			suite.FailNow("Test didn't finish on time")
		}
	}
	err := testConfig.listener.Close(ctx)
	suite.NoError(err)
}

func (suite *serviceBusTopicSuite) publishAndReceiveMessageWithRetryAfter(testConfig publishReceiveTest, event interface{}) {
	ctx := context.Background()
	gotMessage := make(chan bool)

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
		if err != nil {
			fmt.Printf("ERROR: %s", err)
		}
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

func (suite *serviceBusTopicSuite) publishAndReceiveMessageTwice(testConfig publishReceiveTest, event interface{}) {
	ctx := context.Background()
	gotMessage := make(chan bool)
	if testConfig.listenerOptions == nil {
		testConfig.listenerOptions = []listener.Option{}
	}
	if testConfig.publisherOptions == nil {
		testConfig.publisherOptions = []topic.Option{}
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

func (suite *serviceBusTopicSuite) publishAndReceiveMessageWithAutoLockRenewal(testConfig publishReceiveTest, event interface{}) {
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

func (suite *serviceBusTopicSuite) publishAndReceiveMessageNotRenewingLock(testConfig publishReceiveTest, event interface{}) {
	parenrCtx := context.Background()
	returnedHandler := make(chan message.Handler, 1)
	lockRenewalFailureHandler := message.HandleFunc(func(ctx context.Context, msg *message.Message) message.Handler {
		ctx, sp := tab.StartSpan(ctx, "go-shuttle.test.lockrenewalhandler")
		defer sp.End()
		tab.StringAttribute("msg.DeliveryCount", fmt.Sprint(msg.Message().DeliveryCount))
		tab.StringAttribute("msg.LockedUntil", fmt.Sprint(msg.Message().SystemProperties.LockedUntil))

		// the shortlock is set at 1 second in the listener setup
		// We sleep for longer to trigger the lock renewal in the listener.
		// if the renewal works, then the message completion will succeed.
		time.Sleep(5 * time.Second)

		fmt.Printf("[%s] trying to complete!\n", time.Now())
		// Force inline completion to retrieve resulting handler
		res := msg.Complete().Do(ctx, nil, msg.Message())
		// push the returned handler into the channel to trigger the assertion below
		returnedHandler <- res
		return res
	})

	// setup listener
	go func() {
		testConfig.listener.Listen(
			parenrCtx,
			lockRenewalFailureHandler,
			testConfig.topicName,
			testConfig.listenerOptions...,
		)
	}()
	// publish after the listener is setup
	time.Sleep(5 * time.Second)
	err := testConfig.publisher.Publish(
		parenrCtx,
		event,
		testConfig.publisherOptions...,
	)
	suite.NoError(err)

	// expect error handler returned
	select {
	case h := <-returnedHandler:
		// This shows that the handler Complete call returns an error when the lock
		// on the message has expired.
		assert.True(suite.T(), message.IsError(h))
	case <-time.After(12 * time.Second):
		suite.T().Errorf("message never reached the handler or is hanging")
	}
	err = testConfig.listener.Close(parenrCtx)
	time.Sleep(2 * time.Second)
	suite.NoError(err)
}

func (suite *serviceBusTopicSuite) publishAndReceiveMessageWithPrefetch(testConfig publishReceiveTest, event *testEvent) {
	parentCtx := context.Background()
	returnedHandler := make(chan message.Handler, *testConfig.publishCount)
	lockRenewalFailureHandler := message.HandleFunc(func(ctx context.Context, msg *message.Message) message.Handler {
		ctx, sp := tab.StartSpan(ctx, "go-shuttle.test.prefetch")
		defer sp.End()
		tab.StringAttribute("msg.DeliveryCount", fmt.Sprint(msg.Message().DeliveryCount))
		tab.StringAttribute("msg.LockedUntil", fmt.Sprint(msg.Message().SystemProperties.LockedUntil))

		// simulate work
		time.Sleep(20 * time.Second)

		fmt.Printf("[%s] trying to complete ID %s!\n", time.Now().Format(time.RFC3339), msg.Message().ID)
		// Force inline completion to retrieve resulting handler
		res := msg.Complete().Do(ctx, nil, msg.Message())
		// push the returned handler into the channel to trigger the assertion below
		returnedHandler <- res
		return res
	})

	// sets up the subscription so that messages get transferred there, waiting for the listener
	go func() {
		testConfig.listener.Listen(
			parentCtx,
			lockRenewalFailureHandler,
			testConfig.topicName,
			testConfig.listenerOptions...,
		)
	}()
	time.Sleep(2 * time.Second)
	if err := testConfig.listener.Close(parentCtx); err != nil {
		fmt.Printf("error on close is OK here. no message received yet: %s", err)
	}

	// publish all the events
	for i := 0; i < *testConfig.publishCount; i++ {
		event.ID = i
		err := testConfig.publisher.Publish(
			parentCtx,
			event,
			testConfig.publisherOptions...)
		if err != nil {
			suite.T().Errorf(err.Error())
		}
	}

	fmt.Printf("\n!!!!!!!!!!!! PUBLISH DONE. START LISTENING!!!!!!!!!\n")

	// restart the listener
	go func() {
		testConfig.listener.Listen(
			parentCtx,
			lockRenewalFailureHandler,
			testConfig.topicName,
			testConfig.listenerOptions...,
		)
	}()

	totalHandled := 0
	run := true

	// expect only success handler returned
	maxTime := 1 * time.Minute
	timeout := time.NewTimer(maxTime)
	for run {
		select {
		case h := <-returnedHandler:
			// This shows that the handler Complete call returns an error when the lock
			// on the message has expired.
			assert.True(suite.T(), message.IsDone(h), "should return a doneHandler on success")
			if message.IsError(h) {
				suite.T().Errorf("failed to handle message succesfully")
				suite.T().FailNow()
			}
			totalHandled++
			suite.T().Log("successfully processed msg", totalHandled)
			if totalHandled == *testConfig.publishCount {
				run = false
				timeout.Stop()
			}
		case <-timeout.C:
			suite.T().Errorf("took over %s seconds for %d messages", maxTime, *testConfig.publishCount)
			run = false
		}
	}

	err := testConfig.listener.Close(parentCtx)
	if err != nil {
		suite.T().Errorf("failed to close the listener: %s", err)
	}
}

func checkResultHandler(publishedMsg string, publishedMsgType string, ch chan<- bool) message.Handler {
	return message.HandleFunc(
		func(ctx context.Context, msg *message.Message) message.Handler {
			if publishedMsg != msg.Data() {
				errHandler := message.Error(errors.New("published message and received message are different"))
				res := errHandler.Do(ctx, nil, msg.Message()) // Call do to attempt to abandon the message before closing the connection
				ch <- false
				return res
			}
			if publishedMsgType != msg.Type() {
				errHandler := message.Error(errors.New("published message type and received message type are different"))
				res := errHandler.Do(ctx, nil, msg.Message()) // Call do to attempt to abandon the message before closing the connection
				ch <- false
				return res
			}
			if publishedMsgType == reflection.GetType(retryLaterEvent{}) {
				// use delivery count now that retry later abandons
				if msg.Message().DeliveryCount == 2 {
					resHandler := message.Complete().Do(ctx, nil, msg.Message())
					if message.IsDone(resHandler) {
						ch <- true
					} else if message.IsError(resHandler) {
						resHandler = resHandler.Do(ctx, nil, msg.Message())
						ch <- false
					}
					return resHandler
				} else {
					return message.RetryLater(1 * time.Second)
				}
			}
			if publishedMsgType == reflection.GetType(shortLockMessage{}) {
				// the shortlock is set at 1 second in the listener setup
				// We sleep for longer to trigger the lock renewal in the listener.
				// if the renewal works, then the message completion will succeed.
				time.Sleep(3 * time.Second)
				resHandler := message.Complete().Do(ctx, nil, msg.Message()) //if renew failed, complete will fail and we don't return Done().
				if message.IsDone(resHandler) {
					ch <- true
				} else if message.IsError(resHandler) {
					resHandler = resHandler.Do(ctx, nil, msg.Message())
					ch <- false
				}
				return resHandler
			}

			resHandler := message.Complete().Do(ctx, nil, msg.Message())
			if message.IsDone(resHandler) {
				ch <- true
			} else if message.IsError(resHandler) {
				resHandler = resHandler.Do(ctx, nil, msg.Message())
				ch <- false
			}
			return resHandler
		})
}
