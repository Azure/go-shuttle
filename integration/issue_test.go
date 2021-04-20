// +build integration

package integration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/go-shuttle/listener"
	"github.com/Azure/go-shuttle/message"
	"github.com/Azure/go-shuttle/publisher"
	"github.com/devigned/tab"
	"github.com/stretchr/testify/assert"
)

func (suite *serviceBusSuite) TestCompleteCloseToLockExpiryWithPrefetch() {
	suite.T().Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	topic := suite.Prefix + "issue189" + suite.TagID
	pub, err := publisher.New(context.Background(), topic, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := listener.New(
		suite.listenerAuthOption,
		listener.WithSubscriptionDetails(30*time.Second, 3),
		listener.WithSubscriptionName("issue189"))
	suite.NoError(err)
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	publishCount := 10
	suite.verifyHalfOfLockDurationComplete(publishReceiveTest{
		topicName:       topic,
		listener:        l,
		publisher:       pub,
		shouldSucceed:   true,
		publishCount:    &publishCount,
		listenerOptions: []listener.Option{listener.WithMaxConcurrency(3), listener.WithPrefetchCount(3)},
	}, event)
}

func (suite *serviceBusSuite) TestCompleteCloseToLockExpiryNoConcurrency() {
	suite.T().Parallel()
	// creating a separate topic that was not created at the beginning of the test suite
	// note that this topic will also be deleted at the tear down of the suite due to the tagID at the end of the topic name
	topic := suite.Prefix + "issue189" + suite.TagID
	pub, err := publisher.New(context.Background(), topic, suite.publisherAuthOption)
	suite.NoError(err)
	l, err := listener.New(
		suite.listenerAuthOption,
		listener.WithSubscriptionDetails(30*time.Second, 3),
		listener.WithSubscriptionName("issue189"))
	suite.NoError(err)
	event := &testEvent{
		ID:    1,
		Key:   "key",
		Value: "value",
	}
	publishCount := 5
	suite.verifyHalfOfLockDurationComplete(publishReceiveTest{
		topicName:     topic,
		listener:      l,
		publisher:     pub,
		shouldSucceed: true,
		publishCount:  &publishCount,
	}, event)
}

// https://github.com/Azure/azure-service-bus-go/issues/189
func (suite *serviceBusSuite) verifyHalfOfLockDurationComplete(testConfig publishReceiveTest, event *testEvent) {
	parentCtx := context.Background()
	returnedHandler := make(chan message.Handler, *testConfig.publishCount)
	handler := message.HandleFunc(func(ctx context.Context, msg *message.Message) message.Handler {
		ctx, sp := tab.StartSpan(ctx, "go-shuttle.test.handle")
		defer sp.End()
		tab.StringAttribute("msg.DeliveryCount", fmt.Sprint(msg.Message().DeliveryCount))
		tab.StringAttribute("msg.LockedUntil", fmt.Sprint(msg.Message().SystemProperties.LockedUntil))
		fmt.Printf("[%s] handling message ID %s - DeliveryTag : %s\n", time.Now().Format(time.RFC3339), msg.Message().ID, msg.Message().LockToken.String())
		// simulate work for more than 1/2 of lock duration
		time.Sleep(20 * time.Second)

		fmt.Printf("[%s] trying to complete ID: %s - Tag: %s !\n", time.Now().Format(time.RFC3339), msg.Message().ID, msg.Message().LockToken.String())
		// Force inline completion to retrieve resulting handler
		res := msg.Complete().Do(ctx, nil, msg.Message())
		if message.IsError(res) {
			fmt.Printf("[%s] failed to complete ID %s!\n", time.Now().Format(time.RFC3339), msg.Message().ID)
		}
		// push the returned handler into the channel to trigger the assertion below
		returnedHandler <- res
		return res
	})

	// restart the listener
	go func() {
		testConfig.listener.Listen(
			parentCtx,
			handler,
			testConfig.topicName,
			testConfig.listenerOptions...,
		)
	}()

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
				suite.T().Errorf(err.Error())
			}
		}(i)
	}

	wg.Wait()
	fmt.Println("############## DONE SENDING! ###############")

	totalHandled := 0
	run := true

	// expect only success handler returned
	maxTime := 4 * time.Minute
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
