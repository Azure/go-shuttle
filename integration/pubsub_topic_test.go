//go:build integration
// +build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/devigned/tab"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Azure/go-shuttle/internal/reflection"
	"github.com/Azure/go-shuttle/message"
	"github.com/Azure/go-shuttle/topic"
	"github.com/Azure/go-shuttle/topic/listener"
	"github.com/Azure/go-shuttle/topic/publisher"
)

// TestPublishAndListenWithConnectionStringUsingDefault tests both the publisher and listener with default configurations
func (suite *serviceBusTopicSuite) TestPublishAndListenUsingDefault() {
	t := suite.T()
	pub, err := topic.NewPublisher(context.Background(), suite.TopicName, suite.publisherAuthOption)
	require.NoError(t, err)
	l, err := topic.NewListener(suite.listenerAuthOption, listener.WithSubscriptionName("defaultTestSub"))
	require.NoError(t, err)

	defaultTest(t, pub, l, suite.TopicName)
}

// TestPublishAndListenMessageTwice tests publish and listen the same messages twice
func (suite *serviceBusTopicSuite) TestPublishAndListenMessageTwice() {
	t := suite.T()
	pub, err := topic.NewPublisher(context.Background(), suite.TopicName, suite.publisherAuthOption)
	require.NoError(t, err)
	l, err := topic.NewListener(suite.listenerAuthOption, listener.WithSubscriptionName("testTwoMessages"))
	require.NoError(t, err)

	defaultTestWithMessageTwice(t, pub, l, suite.TopicName)
}

// TestPublishAndListenWithConnectionStringUsingTypeFilter tests both the publisher and listener with a filter on the event type
func (suite *serviceBusTopicSuite) TestPublishAndListenUsingFilter() {
	t := suite.T()
	pub, err := topic.NewPublisher(context.Background(), suite.TopicName, suite.publisherAuthOption)
	require.NoError(t, err)
	successListener, err := topic.NewListener(suite.listenerAuthOption,
		listener.WithSubscriptionName("subTypeFilter"),
		listener.WithFilterDescriber("testFilter", servicebus.SQLFilter{Expression: "type LIKE 'testEvent'"}))
	require.NoError(t, err)
	failListener, err := topic.NewListener(suite.listenerAuthOption,
		listener.WithSubscriptionName("subTypeFilter"),
		listener.WithFilterDescriber("testFilter", servicebus.SQLFilter{Expression: "type LIKE 'nottestEvent'"}))

	typeFilterTest(t, pub, successListener, suite.TopicName, true)
	typeFilterTest(t, pub, failListener, suite.TopicName, false)
}

type notTestEvent struct {
	wrongType string
}

// TestPublishAndListenWithConnectionStringUsingTypeFilter tests both the publisher and listener with a filter on the event type
func (suite *serviceBusTopicSuite) TestPublishAndListenUsingTypeFilter() {
	t := suite.T()
	pub, err := topic.NewPublisher(context.Background(), suite.TopicName, suite.publisherAuthOption)
	require.NoError(t, err)
	// listener with wrong event type. not getting the event
	failListener, err := topic.NewListener(suite.listenerAuthOption,
		listener.WithSubscriptionName("subEventTypeFilterFail"),
		listener.WithTypeFilter(&notTestEvent{}))
	require.NoError(t, err)
	typeFilterTest(t, pub, failListener, suite.TopicName, false)

	// update subscription to filter on correct event type. succeeds receiving event
	successListener, err := topic.NewListener(suite.listenerAuthOption,
		listener.WithSubscriptionName("subEventTypeFilter"),
		listener.WithTypeFilter(&testEvent{}))
	typeFilterTest(t, pub, successListener, suite.TopicName, true)
}

// TestPublishAndListenUsingCustomHeaderFilter tests both the publisher and listener with a customer filter
func (suite *serviceBusTopicSuite) TestPublishAndListenUsingCustomHeaderFilter() {
	t := suite.T()
	// this assumes that the testTopic was created at the start of the test suite
	pub, err := topic.NewPublisher(
		context.Background(),
		suite.TopicName,
		suite.publisherAuthOption,
		publisher.SetDefaultHeader("testHeader", "Key"))
	require.NoError(t, err)
	successListener, err := topic.NewListener(suite.listenerAuthOption,
		listener.WithSubscriptionName("subNameHeader"),
		listener.WithFilterDescriber("testFilter", servicebus.SQLFilter{Expression: "testHeader LIKE 'key'"}))
	require.NoError(t, err)
	failListener, err := topic.NewListener(suite.listenerAuthOption,
		listener.WithSubscriptionName("subNameHeader"),
		listener.WithFilterDescriber("testFilter", servicebus.SQLFilter{Expression: "testHeader LIKE 'notkey'"}))
	require.NoError(t, err)

	suite.customHeaderFilterTest(pub, successListener, true)
	suite.customHeaderFilterTest(pub, failListener, false)
}

// TestPublishAndListenWithConnectionStringUsingDuplicateDetection tests both the publisher and listener with duplicate detection
func (suite *serviceBusTopicSuite) TestPublishAndListenUsingDuplicateDetection() {
	t := suite.T()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	dupeDetectionTopicName := suite.Prefix + "deduptopic" + suite.TagID
	dupeDetectionWindow := 5 * time.Minute
	pub, err := topic.NewPublisher(
		context.Background(),
		dupeDetectionTopicName,
		suite.publisherAuthOption,
		publisher.WithDuplicateDetection(&dupeDetectionWindow))
	require.NoError(t, err)
	l, err := topic.NewListener(suite.listenerAuthOption, listener.WithSubscriptionName("subDedup"))
	require.NoError(t, err)
	duplicateDetectionTest(t, pub, l, dupeDetectionTopicName)
}

func (suite *serviceBusTopicSuite) TestPublishAndListenRetryLater() {
	t := suite.T()
	t.Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	retryLaterTopic := suite.Prefix + "retrylater" + suite.TagID
	pub, err := topic.NewPublisher(context.Background(), retryLaterTopic, suite.publisherAuthOption)
	require.NoError(t, err)
	l, err := topic.NewListener(
		suite.listenerAuthOption,
		listener.WithSubscriptionName("subRetryLater"))
	require.NoError(t, err)
	// create retryLater event. listener emits retry based on event type
	event := &retryLaterEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	publishAndReceiveMessageWithRetryAfter(t, publishReceiveTest{
		topicName:       retryLaterTopic,
		listener:        l,
		publisher:       pub,
		listenerOptions: []listener.Option{},
		shouldSucceed:   true,
	}, event)
}

func (suite *serviceBusTopicSuite) TestPublishAndListenShortLockDuration() {
	t := suite.T()
	t.Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	shortLockTopic := suite.Prefix + "shortlock" + suite.TagID
	pub, err := topic.NewPublisher(context.Background(), shortLockTopic, suite.publisherAuthOption)
	require.NoError(t, err)
	l, err := topic.NewListener(
		suite.listenerAuthOption,
		listener.WithSubscriptionDetails(2*time.Second, 3),
		listener.WithSubscriptionName("subshortlock"))
	require.NoError(t, err)
	// create retryLater event. listener emits retry based on event type
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	publishAndReceiveMessageWithAutoLockRenewal(t, publishReceiveTest{
		topicName:       shortLockTopic,
		listener:        l,
		publisher:       pub,
		listenerOptions: []listener.Option{listener.WithMessageLockAutoRenewal(1 * time.Second)},
		shouldSucceed:   true,
		receiveTimeout:  10 * time.Second,
	}, event)
}

func (suite *serviceBusTopicSuite) TestPublishAndListenRenewLockPassedClaimValidity() {
	// suite.T().Skip()
	t := suite.T()
	t.Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	shortLockTopic := suite.Prefix + "locktestclaim" + suite.TagID
	pub, err := topic.NewPublisher(context.Background(), shortLockTopic, suite.publisherAuthOption)
	require.NoError(t, err)
	l, err := topic.NewListener(
		suite.listenerAuthOption,
		listener.WithSubscriptionDetails(30*time.Second, 1),
		listener.WithSubscriptionName("subclaimtest"))
	require.NoError(t, err)
	// create event. listener emits retry based on event type
	event := &customLockMessage{
		SleepMinutes: 16,
	}
	publishCount := 500
	publishAndReceiveMessageWithAutoLockRenewal(t, publishReceiveTest{
		topicName: shortLockTopic,
		listener:  l,
		publisher: pub,
		listenerOptions: []listener.Option{
			listener.WithMessageLockAutoRenewal(5 * time.Second),
			listener.WithPrefetchCount(500),
			listener.WithMaxConcurrency(500),
		},
		shouldSucceed:  true,
		publishCount:   &publishCount,
		receiveTimeout: 17 * time.Minute,
	}, event)
}

func (suite *serviceBusTopicSuite) TestPublishAndListenNotRenewingLock() {
	t := suite.T()
	t.Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	norenewlockTopic := suite.Prefix + "norenewlock" + suite.TagID
	pub, err := topic.NewPublisher(context.Background(), norenewlockTopic, suite.publisherAuthOption)
	require.NoError(t, err)
	l, err := topic.NewListener(
		suite.listenerAuthOption,
		listener.WithSubscriptionDetails(2*time.Second, 2),
		listener.WithSubscriptionName("norenewlock"))
	require.NoError(t, err)
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	publishAndReceiveMessageNotRenewingLock(t, publishReceiveTest{
		topicName:       norenewlockTopic,
		listener:        l,
		publisher:       pub,
		listenerOptions: []listener.Option{},
		shouldSucceed:   false,
	}, event)
}

func (suite *serviceBusTopicSuite) TestPublishAndListenConcurrentPrefetch() {
	t := suite.T()
	t.Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	prefetchTopic := suite.Prefix + "prefetch" + suite.TagID
	pub, err := topic.NewPublisher(context.Background(), prefetchTopic, suite.publisherAuthOption)
	require.NoError(t, err)
	l, err := topic.NewListener(
		suite.listenerAuthOption,
		listener.WithSubscriptionDetails(60*time.Second, 1),
		listener.WithSubscriptionName("prefetch"))
	require.NoError(t, err)
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	publishCount := 2000
	publishAndReceiveMessageWithPrefetch(t, publishReceiveTest{
		topicName: prefetchTopic,
		listener:  l,
		publisher: pub,
		listenerOptions: []listener.Option{
			listener.WithPrefetchCount(250),
			listener.WithMessageLockAutoRenewal(20 * time.Second),
			listener.WithMaxConcurrency(250),
		},
		shouldSucceed: true,
		publishCount:  &publishCount,
	}, event)
}

func defaultTest(t *testing.T, p *publisher.Publisher, l *listener.Listener, topicName string) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	publishAndReceiveMessage(t,
		publishReceiveTest{
			topicName:     topicName,
			listener:      l,
			publisher:     p,
			shouldSucceed: true,
		},
		event,
	)
}

func defaultTestWithMessageTwice(t *testing.T, p *publisher.Publisher, l *listener.Listener, topicName string) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key1",
		Value: "value1",
	}
	publishAndReceiveMessageTwice(t,
		publishReceiveTest{
			topicName:     topicName,
			listener:      l,
			publisher:     p,
			shouldSucceed: true,
		},
		event,
	)
}

func typeFilterTest(t *testing.T, p *publisher.Publisher, l *listener.Listener, topicName string, shouldSucceed bool) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with a filter on the event type
	publishAndReceiveMessage(t,
		publishReceiveTest{
			topicName:     topicName,
			listener:      l,
			publisher:     p,
			shouldSucceed: shouldSucceed,
		},
		event,
	)
}

func (suite *serviceBusTopicSuite) customHeaderFilterTest(pub *publisher.Publisher, l *listener.Listener, shouldSucceed bool) {
	t := suite.T()
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with a filter on the custom header
	publishAndReceiveMessage(t,
		publishReceiveTest{
			topicName:     suite.TopicName,
			listener:      l,
			publisher:     pub,
			shouldSucceed: shouldSucceed,
		},
		event,
	)
}

func duplicateDetectionTest(t *testing.T, pub *publisher.Publisher, l *listener.Listener, topicName string) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with duplicate detection
	publishCount := 2
	publishAndReceiveMessage(t,
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
	publishAndReceiveMessage(t,
		publishReceiveTest{
			topicName:     topicName,
			listener:      l,
			publisher:     pub,
			shouldSucceed: true,
		},
		event2,
	)
}

func publishAndReceiveMessage(t *testing.T, testConfig publishReceiveTest, event interface{}) {
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
		eventBytes, err := testConfig.publisher.Marshaller().Marshal(event)
		require.NoError(t, err)
		err = testConfig.listener.Listen(
			ctx,
			checkResultHandler(string(eventBytes), reflection.GetType(testEvent{}), gotMessage),
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
		require.NoError(t, err)
	}

	select {
	case isSuccessful := <-gotMessage:
		if testConfig.shouldSucceed != isSuccessful {
			t.Log("Test did not succeed")
			t.FailNow()
		}
	case <-time.After(15 * time.Second):
		if testConfig.shouldSucceed {
			t.Log("Test didn't finish on time")
			t.FailNow()
		}
	}
	err := testConfig.listener.Close(ctx)
	require.NoError(t, err)
}

func publishAndReceiveMessageWithRetryAfter(t *testing.T, testConfig publishReceiveTest, event interface{}) {
	ctx := context.Background()
	gotMessage := make(chan bool)

	// setup listener
	go func() {
		eventBytes, err := testConfig.publisher.Marshaller().Marshal(event)
		require.NoError(t, err)
		err = testConfig.listener.Listen(
			ctx,
			checkResultHandler(string(eventBytes), reflection.GetType(event), gotMessage),
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
		require.NoError(t, err)
	}

	select {
	case isSuccessful := <-gotMessage:
		if testConfig.shouldSucceed != isSuccessful {
			t.FailNow()
		}
		t.Log("Message succesfully completed")
	case <-time.After(10 * time.Second):
		if testConfig.shouldSucceed {
			t.Log("Test didn't finish on time")
			t.FailNow()
		}
	}
	err := testConfig.listener.Close(ctx)
	require.NoError(t, err)
}

func publishAndReceiveMessageTwice(t *testing.T, testConfig publishReceiveTest, event interface{}) {
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
		eventBytes, err := testConfig.publisher.Marshaller().Marshal(event)
		require.NoError(t, err)
		err = testConfig.listener.Listen(
			ctx,
			checkResultHandler(string(eventBytes), reflection.GetType(event), gotMessage),
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
	require.NoError(t, err)

	select {
	case isSuccessful := <-gotMessage:
		if testConfig.shouldSucceed != isSuccessful {
			t.FailNow()
		}
	case <-time.After(10 * time.Second):
		if testConfig.shouldSucceed {
			t.Log("Test didn't finish on time")
			t.FailNow()
		}
	}

	// publish same message again
	time.Sleep(5 * time.Second)
	err = testConfig.publisher.Publish(
		ctx,
		event,
		testConfig.publisherOptions...,
	)
	require.NoError(t, err)

	select {
	case isSuccessful := <-gotMessage:
		if testConfig.shouldSucceed != isSuccessful {
			t.FailNow()
		}
	case <-time.After(10 * time.Second):
		if testConfig.shouldSucceed {
			t.Log("Test didn't finish on time")
			t.FailNow()
		}
	}

	err = testConfig.listener.Close(ctx)
	require.NoError(t, err)
}

func publishAndReceiveMessageWithAutoLockRenewal(t *testing.T, testConfig publishReceiveTest, event interface{}) {
	sendCtx := context.Background()
	listenCtx := context.Background()
	gotMessage := make(chan bool)

	// setup listener
	go func() {
		eventBytes, err := testConfig.publisher.Marshaller().Marshal(event)
		require.NoError(t, err)
		testConfig.listener.Listen(
			listenCtx,
			checkResultHandler(string(eventBytes), reflection.GetType(event), gotMessage),
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
		go func() {
			err := testConfig.publisher.Publish(
				sendCtx,
				event,
				testConfig.publisherOptions...,
			)
			require.NoError(t, err)
		}()
	}
	completedCount := 0
	deadline := time.After(testConfig.receiveTimeout)
	for completedCount < publishCount {
		select {
		case isSuccessful := <-gotMessage:
			t.Log("Processed message ", completedCount)
			if testConfig.shouldSucceed != isSuccessful {
				t.Fail()
				completedCount = publishCount // exit
			} else {
				completedCount++
				t.Log("Message successfully completed ", completedCount)
			}
		case <-deadline:
			if testConfig.shouldSucceed {
				t.Log("Message successfully completed ", completedCount)
				t.FailNow()
			}
			completedCount = publishCount // exit
		}
	}
	err := testConfig.listener.Close(listenCtx)
	require.NoError(t, err)
}

func publishAndReceiveMessageNotRenewingLock(t *testing.T, testConfig publishReceiveTest, event interface{}) {
	parenrCtx := context.Background()
	returnedHandler := make(chan message.Handler, 1)
	lockRenewalFailureHandler := message.HandleFunc(func(ctx context.Context, msg *message.Message) message.Handler {
		ctx, sp := tab.StartSpan(ctx, "go-shuttle.test.lockrenewalhandler")
		defer sp.End()
		tab.StringAttribute("msg.DeliveryCount", fmt.Sprint(msg.Message().DeliveryCount))
		tab.StringAttribute("msg.LockedUntil", fmt.Sprint(msg.Message().SystemProperties.LockedUntil))
		t.Logf("[%s] msg.DeliveryCount: %d \n", time.Now().In(time.UTC), msg.Message().DeliveryCount)
		t.Logf("[%s] msg.LockedUntil: %s \n", time.Now().In(time.UTC), msg.Message().SystemProperties.LockedUntil)

		// the shortlock is set at 1 second in the listener setup
		// We sleep for longer to trigger the lock renewal in the listener.
		// if the renewal works, then the message completion will succeed.
		time.Sleep(10 * time.Second)

		t.Logf("[%s] trying to complete!\n", time.Now())
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
	require.NoError(t, err)

	// expect error handler returned
	select {
	case h := <-returnedHandler:
		// This shows that the handler Complete call returns an error when the lock
		// on the message has expired.
		assert.True(t, message.IsError(h))
	case <-time.After(12 * time.Second):
		t.Errorf("message never reached the handler or is hanging")
	}
	err = testConfig.listener.Close(parenrCtx)
	time.Sleep(2 * time.Second)
	require.NoError(t, err)
}

func publishAndReceiveMessageWithPrefetch(t *testing.T, testConfig publishReceiveTest, event *testEvent) {
	parentCtx := context.Background()
	returnedHandler := make(chan message.Handler, *testConfig.publishCount)
	lockRenewalFailureHandler := message.HandleFunc(func(ctx context.Context, msg *message.Message) message.Handler {
		ctx, sp := tab.StartSpan(ctx, "go-shuttle.test.prefetch")
		defer sp.End()
		tab.StringAttribute("msg.DeliveryCount", fmt.Sprint(msg.Message().DeliveryCount))
		tab.StringAttribute("msg.LockedUntil", fmt.Sprint(msg.Message().SystemProperties.LockedUntil))

		// simulate work
		time.Sleep(10 * time.Second)

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
	time.Sleep(5 * time.Second)

	// publish all the events
	for i := 0; i < *testConfig.publishCount; i++ {
		event.ID = i
		go func(e testEvent) {
			err := testConfig.publisher.Publish(
				parentCtx,
				event,
				testConfig.publisherOptions...)
			if err != nil {
				t.Errorf(err.Error())
			}
			t.Log("Published event", e.ID)
		}(*event)
	}

	fmt.Printf("\n!!!!!!!!!!!! PUBLISH DONE. START LISTENING!!!!!!!!!\n")

	totalHandled := 0
	run := true

	// expect only success handler returned
	maxTime := 5 * time.Minute
	timeout := time.NewTimer(maxTime)
	for run {
		select {
		case h := <-returnedHandler:
			// This shows that the handler Complete call returns an error when the lock
			// on the message has expired.
			assert.True(t, message.IsDone(h), "should return a doneHandler on success")
			if message.IsError(h) {
				t.Errorf("failed to handle message succesfully")
				t.FailNow()
			}
			totalHandled++
			t.Log("successfully processed msg", totalHandled)
			if totalHandled == *testConfig.publishCount {
				run = false
				timeout.Stop()
			}
		case <-timeout.C:
			t.Errorf("took over %s seconds for %d messages", maxTime, *testConfig.publishCount)
			run = false
		}
	}

	err := testConfig.listener.Close(parentCtx)
	if err != nil {
		t.Errorf("failed to close the listener: %s", err)
	}
}

type customLockMessage struct {
	SleepMinutes int
}

func checkResultHandler(publishedMsg string, publishedMsgType string, ch chan<- bool) message.Handler {
	return message.HandleFunc(
		func(ctx context.Context, msg *message.Message) message.Handler {
			if publishedMsg != msg.Data() {
				errHandler := message.Error(errors.New("published message and received message are different"))
				res := errHandler.Do(ctx, nil, msg.Message()) // Call do to attempt to abandon the message before closing the connection
				fmt.Println("fail data equality")
				ch <- false
				return res
			}
			if publishedMsgType != msg.Type() {
				errHandler := message.Error(errors.New("published message type and received message type are different"))
				res := errHandler.Do(ctx, nil, msg.Message()) // Call do to attempt to abandon the message before closing the connection
				fmt.Println("fail type equality")
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
			if publishedMsgType == reflection.GetType(userPropertyEvent{}) {
				userProp, ok := msg.Message().UserProperties["testProperty"]
				if !ok {
					fmt.Println("fail get from user property")
					res := message.Complete().Do(ctx, nil, msg.Message())
					ch <- false
					return res
				}
				if msg.Message().Label != "LabelSet" {
					fmt.Printf("label : %s\n", msg.Message().Label)
					fmt.Println("fail label or user prop equality")
					res := message.Complete().Do(ctx, nil, msg.Message())
					ch <- false
					return res
				}
				if userProp.(int64) != 1 {
					fmt.Printf("type : %T\n", userProp)
					fmt.Printf("prop : %v\n", msg.Message().UserProperties)
					fmt.Println("fail user prop equality")
					res := message.Complete().Do(ctx, nil, msg.Message())
					ch <- false
					return res
				}
				res := message.Complete().Do(ctx, nil, msg.Message())
				ch <- true
				return res
			}
			if publishedMsgType == reflection.GetType(shortLockMessage{}) {
				// the shortlock is set at 1 second in the listener setup
				// We sleep for longer to trigger the lock renewal in the listener.
				// if the renewal works, then the message completion will succeed.
				time.Sleep(3 * time.Second)
				resHandler := message.Complete().Do(ctx, nil, msg.Message()) // if renew failed, complete will fail and we don't return Done().
				if message.IsDone(resHandler) {
					ch <- true
				} else if message.IsError(resHandler) {
					resHandler = resHandler.Do(ctx, nil, msg.Message())
					ch <- false
				}
				return resHandler
			}
			if publishedMsgType == reflection.GetType(customLockMessage{}) {
				// the shortlock is set at 1 second in the listener setup
				// We sleep for longer to trigger the lock renewal in the listener.
				// if the renewal works, then the message completion will succeed.
				fmt.Println("=== Handling message ===")
				lockMessage := &customLockMessage{}
				err := msg.Unmarshal(msg.Message().Data, lockMessage)
				if err != nil {
					fmt.Println("=== failed unmarshalling :", err)
					ch <- false
					return message.Complete().Do(ctx, nil, msg.Message())
				}
				sleepTime := time.Duration(int64(lockMessage.SleepMinutes) * int64(time.Minute))
				fmt.Println("=== sleeping for ", sleepTime)
				time.Sleep(sleepTime)
				fmt.Println("=== done sleeping! ")
				resHandler := message.Complete().Do(ctx, nil, msg.Message()) // if renew failed, complete will fail and we don't return Done().
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
