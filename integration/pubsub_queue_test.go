//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/devigned/tab"
	"github.com/stretchr/testify/assert"

	"github.com/Azure/go-shuttle/integration/protomsg"
	"github.com/Azure/go-shuttle/internal/reflection"
	"github.com/Azure/go-shuttle/marshal"
	"github.com/Azure/go-shuttle/message"
	"github.com/Azure/go-shuttle/queue"
	"github.com/Azure/go-shuttle/queue/listener"
	"github.com/Azure/go-shuttle/queue/publisher"
)

// TestPublishAndListenWithConnectionStringUsingDefault tests both the publisher and listener with default configurations
func (suite *serviceBusQueueSuite) TestPublishAndListenUsingDefault() {
	pub, err := queue.NewPublisher(context.Background(), suite.QueueName, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := queue.NewListener(suite.listenerAuthOption)
	suite.NoError(err)

	suite.defaultTest(pub, l)
}

// TestPublishAndListenWithConnectionStringUsingDefault tests both the publisher and listener with default configurations
func (suite *serviceBusQueueSuite) TestPublishAndListenProtoMessage() {
	pub, err := queue.NewPublisher(context.Background(), suite.QueueName, suite.publisherAuthOption)
	pub.SetMarshaller(marshal.ProtobufMarshaller)
	suite.NoError(err)
	l, err := queue.NewListener(suite.listenerAuthOption)
	suite.NoError(err)

	suite.defaultTest(pub, l)
}

// TestPublishAndListenMessageTwice tests publish and listen the same messages twice
func (suite *serviceBusQueueSuite) TestPublishAndListenMessageTwice() {
	pub, err := queue.NewPublisher(context.Background(), suite.QueueName, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := queue.NewListener(suite.listenerAuthOption)
	suite.NoError(err)

	suite.defaultTestWithMessageTwice(pub, l)
}

// TestPublishAndListenWithConnectionStringUsingDuplicateDetection tests both the publisher and listener with duplicate detection
func (suite *serviceBusQueueSuite) TestPublishAndListenUsingDuplicateDetection() {
	suite.T().Parallel()
	// creating a separate queue that was not created at the beginning of the test suite
	// note that this queue will also be deleted at the tear down of the suite due to the tagID at the end of the queue name
	dupeDetectionQueueName := suite.Prefix + "dedupqueue" + suite.TagID
	dupeDetectionWindow := 5 * time.Minute
	pub, err := queue.NewPublisher(
		context.Background(),
		dupeDetectionQueueName,
		suite.publisherAuthOption,
		publisher.WithDuplicateDetection(&dupeDetectionWindow))
	suite.NoError(err)
	l, err := queue.NewListener(suite.listenerAuthOption)
	suite.NoError(err)
	suite.duplicateDetectionTest(pub, l, dupeDetectionQueueName)
}

func (suite *serviceBusQueueSuite) TestPublishAndListenRetryLater() {
	suite.T().Parallel()
	// creating a separate queue that was not created at the beginning of the test suite
	// note that this queue will also be deleted at the tear down of the suite due to the tagID at the end of the queue name
	retryLaterQueue := suite.Prefix + "retrylater" + suite.TagID
	pub, err := queue.NewPublisher(context.Background(), retryLaterQueue, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := queue.NewListener(
		suite.listenerAuthOption)
	suite.NoError(err)
	// create retryLater event. listener emits retry based on event type
	event := &retryLaterEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	suite.publishAndReceiveMessageWithRetryAfter(publishReceiveQueueTest{
		queueName:       retryLaterQueue,
		listener:        l,
		publisher:       pub,
		listenerOptions: []listener.Option{},
		shouldSucceed:   true,
	}, event)
}

func (suite *serviceBusQueueSuite) TestPublishAndListenShortLockDuration() {
	suite.T().Parallel()
	// creating a separate queue that was not created at the beginning of the test suite
	// note that this queue will also be deleted at the tear down of the suite due to the tagID at the end of the queue name
	shortLockQueue := suite.Prefix + "shortlock" + suite.TagID
	pub, err := queue.NewPublisher(context.Background(), shortLockQueue, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := queue.NewListener(
		suite.listenerAuthOption)
	suite.NoError(err)
	// create retryLater event. listener emits retry based on event type
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	suite.publishAndReceiveMessageWithAutoLockRenewal(publishReceiveQueueTest{
		queueName:       shortLockQueue,
		listener:        l,
		publisher:       pub,
		listenerOptions: []listener.Option{listener.WithMessageLockAutoRenewal(1 * time.Second)},
		shouldSucceed:   true,
	}, event)
}

func (suite *serviceBusQueueSuite) defaultTest(p *publisher.Publisher, l *listener.Listener) {
	// create test event
	event := &protomsg.TestEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	suite.publishAndReceiveMessage(
		publishReceiveQueueTest{
			queueName:     suite.QueueName,
			listener:      l,
			publisher:     p,
			shouldSucceed: true,
		},
		event,
	)
}

func (suite *serviceBusQueueSuite) defaultTestWithMessageTwice(p *publisher.Publisher, l *listener.Listener) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key1",
		Value: "value1",
	}
	suite.publishAndReceiveMessageTwice(
		publishReceiveQueueTest{
			queueName:     suite.QueueName,
			listener:      l,
			publisher:     p,
			shouldSucceed: true,
		},
		event,
	)
}

func (suite *serviceBusQueueSuite) typeFilterTest(p *publisher.Publisher, l *listener.Listener, shouldSucceed bool) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with a filter on the event type
	suite.publishAndReceiveMessage(
		publishReceiveQueueTest{
			queueName:     suite.QueueName,
			listener:      l,
			publisher:     p,
			shouldSucceed: shouldSucceed,
		},
		event,
	)
}

func (suite *serviceBusQueueSuite) customHeaderFilterTest(pub *publisher.Publisher, l *listener.Listener, shouldSucceed bool) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with a filter on the custom header
	suite.publishAndReceiveMessage(
		publishReceiveQueueTest{
			queueName:     suite.QueueName,
			listener:      l,
			publisher:     pub,
			shouldSucceed: shouldSucceed,
		},
		event,
	)
}

func (suite *serviceBusQueueSuite) duplicateDetectionTest(pub *publisher.Publisher, l *listener.Listener, queueName string) {
	// create test event
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	// test with duplicate detection
	publishCount := 2
	suite.publishAndReceiveMessage(
		publishReceiveQueueTest{
			queueName:        queueName,
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
		publishReceiveQueueTest{
			queueName:     queueName,
			listener:      l,
			publisher:     pub,
			shouldSucceed: true,
		},
		event2,
	)
}

func (suite *serviceBusQueueSuite) publishAndReceiveMessage(testConfig publishReceiveQueueTest, event interface{}) {
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
		suite.T().Logf(" %s starting listener for", suite.T().Name())
		eventBytes, err := testConfig.publisher.Marshaller().Marshal(event)
		suite.NoError(err)
		err = testConfig.listener.Listen(
			ctx,
			checkResultHandler(string(eventBytes), reflection.GetType(event), gotMessage),
			testConfig.queueName,
			testConfig.listenerOptions...,
		)
	}()
	publishCount := 1
	if testConfig.publishCount != nil {
		publishCount = *testConfig.publishCount
	}
	for i := 0; i < publishCount; i++ {
		suite.T().Logf("%s - publishing msg %d", suite.T().Name(), i)
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

func (suite *serviceBusQueueSuite) publishAndReceiveMessageWithRetryAfter(testConfig publishReceiveQueueTest, event interface{}) {
	ctx := context.Background()
	gotMessage := make(chan bool)

	// setup listener
	go func() {
		eventJSON, err := json.Marshal(event)
		suite.NoError(err)
		err = testConfig.listener.Listen(
			ctx,
			checkResultHandler(string(eventJSON), reflection.GetType(event), gotMessage),
			testConfig.queueName,
			testConfig.listenerOptions...,
		)
		if err != nil {
			fmt.Printf("ERROR: %s", err)
		}
	}()
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

func (suite *serviceBusQueueSuite) publishAndReceiveMessageTwice(testConfig publishReceiveQueueTest, event interface{}) {
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
			testConfig.queueName,
			testConfig.listenerOptions...,
		)
	}()

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

func (suite *serviceBusQueueSuite) publishAndReceiveMessageWithAutoLockRenewal(testConfig publishReceiveQueueTest, event interface{}) {
	ctx := context.Background()
	gotMessage := make(chan bool)

	// setup listener
	go func() {
		eventJSON, err := json.Marshal(event)
		suite.NoError(err)
		testConfig.listener.Listen(
			ctx,
			checkResultHandler(string(eventJSON), reflection.GetType(event), gotMessage),
			testConfig.queueName,
			testConfig.listenerOptions...,
		)
	}()
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

func (suite *serviceBusQueueSuite) publishAndReceiveMessageNotRenewingLock(testConfig publishReceiveQueueTest, event interface{}) {
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
			testConfig.queueName,
			testConfig.listenerOptions...,
		)
	}()
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

func (suite *serviceBusQueueSuite) publishAndReceiveMessageWithPrefetch(testConfig publishReceiveQueueTest, event *testEvent) {
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
			testConfig.queueName,
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
			testConfig.queueName,
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
