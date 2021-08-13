// +build integration

package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/go-shuttle/topic"
	"github.com/Azure/go-shuttle/topic/listener"
	"github.com/stretchr/testify/require"

	"github.com/Azure/go-shuttle/message"
	"github.com/devigned/tab"
	"github.com/stretchr/testify/assert"
)

func (suite *serviceBusTopicSuite) TestCompleteCloseToLockExpiryWithPrefetch() {
	t := suite.T()
	// creating a separate topicName that was not created at the beginning of the test suite
	// note that this topicName will also be deleted at the tear down of the suite due to the tagID at the end of the topicName name
	topicName := suite.Prefix + "issue189" + suite.TagID
	pub, err := topic.NewPublisher(context.Background(), topicName, suite.publisherAuthOption)
	require.NoError(t, err)
	l, err := topic.NewListener(
		suite.listenerAuthOption,
		listener.WithSubscriptionDetails(30*time.Second, 1),
		listener.WithSubscriptionName("issue189"))
	require.NoError(t, err)
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	publishCount := 10
	verifyHalfOfLockDurationComplete(t, publishReceiveTest{
		topicName:       topicName,
		listener:        l,
		publisher:       pub,
		shouldSucceed:   true,
		publishCount:    &publishCount,
		listenerOptions: []listener.Option{listener.WithMaxConcurrency(5), listener.WithPrefetchCount(5)},
	}, event)
}

func (suite *serviceBusTopicSuite) TestCompleteCloseToLockExpiryNoConcurrency() {
	t := suite.T()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	topicName := suite.Prefix + "issue189" + suite.TagID
	pub, err := topic.NewPublisher(context.Background(), topicName, suite.publisherAuthOption)
	require.NoError(t, err)
	l, err := topic.NewListener(
		suite.listenerAuthOption,
		listener.WithSubscriptionDetails(30*time.Second, 1),
		listener.WithSubscriptionName("issue189"))
	require.NoError(t, err)
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	publishCount := 5
	verifyHalfOfLockDurationComplete(t, publishReceiveTest{
		topicName:     topicName,
		listener:      l,
		publisher:     pub,
		shouldSucceed: true,
		publishCount:  &publishCount,
	}, event)
}

// https://github.com/Azure/azure-service-bus-go/issues/189
func verifyHalfOfLockDurationComplete(t *testing.T, testConfig publishReceiveTest, event *testEvent) {
	parentCtx := context.Background()
	returnedHandler := make(chan message.Handler, *testConfig.publishCount)
	handler := message.HandleFunc(func(ctx context.Context, msg *message.Message) message.Handler {
		ctx, sp := tab.StartSpan(ctx, "go-shuttle.test.handle")
		defer sp.End()
		t.Logf("msg.DeliveryCount: %d - msg.LockedUntil %s", msg.Message().DeliveryCount, msg.Message().SystemProperties.LockedUntil)
		t.Logf("[%s] handling message ID %s - DeliveryTag : %s\n", time.Now().Format(time.RFC3339), msg.Message().ID, msg.Message().LockToken.String())
		// simulate work for more than 1/2 of lock duration
		time.Sleep(20 * time.Second)

		t.Logf("[%s] completing ID: %s - Tag: %s !\n", time.Now().Format(time.RFC3339), msg.Message().ID, msg.Message().LockToken.String())
		// Force inline completion to retrieve resulting handler
		res := msg.Complete().Do(ctx, nil, msg.Message())
		if message.IsError(res) {
			t.Logf("[%s] failed to complete ID %s!\n", time.Now().Format(time.RFC3339), msg.Message().ID)
		}
		// push the returned handler into the channel to trigger the assertion below
		returnedHandler <- res
		return res
	})

	// start the listener
	go func() {
		testConfig.listener.Listen(
			parentCtx,
			handler,
			testConfig.topicName,
			testConfig.listenerOptions...,
		)
	}()

	time.Sleep(10 * time.Second)

	wg := sync.WaitGroup{}

	// publish all the events
	for i := 0; i < *testConfig.publishCount; i++ {
		wg.Add(1)
		go func(it int) {
			event.ID = it
			err := testConfig.publisher.Publish(
				parentCtx,
				event,
				testConfig.publisherOptions...)
			wg.Done()
			if err != nil {
				t.Errorf(err.Error())
			}
		}(i)
	}

	wg.Wait()
	t.Logf("############## DONE SENDING! ###############")

	var totalHandled int32 = 0
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
				run = false
				t.FailNow()
			}
			handled := atomic.AddInt32(&totalHandled, 1)
			t.Logf("successfully processed %d msg", handled)
			if handled == int32(*testConfig.publishCount) {
				if !timeout.Stop() {
					<-timeout.C
				}
				t.Logf("received all %d messages. stopping the loop!", handled)
				run = false
			}
		case <-timeout.C:
			if totalHandled != int32(*testConfig.publishCount) {
				t.Logf("timeout triggered: handled: %d, but expected %d", totalHandled, *testConfig.publishCount)
				t.Errorf("took over %s seconds for %d messages", maxTime, *testConfig.publishCount)
			}
			run = false
		}
	}

	err := testConfig.listener.Close(parentCtx)
	if err != nil {
		t.Errorf("failed to close the listener: %s", err)
	}
}
