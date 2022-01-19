// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/go-shuttle/queue"
	"github.com/Azure/go-shuttle/queue/publisher"
	"github.com/stretchr/testify/require"
)

// TestCreatePublisherWithNewQueue tests the creation of a publisher for a new queue
func (suite *serviceBusQueueSuite) TestCreatePublisherUsingNewQueue() {
	suite.T().Parallel()
	queueName := "newQueue" + suite.TagID
	_, err := queue.NewPublisher(context.Background(), queueName, suite.publisherAuthOption)
	if suite.NoError(err) {
		// make sure that queue exists
		ns := suite.GetNewNamespace()
		tm := ns.NewQueueManager()
		_, err := tm.Get(context.Background(), queueName)
		require.NoError(suite.T(), err)

		// delete new queue
		err = tm.Delete(context.Background(), queueName)
		require.NoError(suite.T(), err)
	}
}

// TestCreatePublisherWithExistingQueue tests the creation of a publisher for an existing queue and a connection string
func (suite *serviceBusQueueSuite) TestCreatePublisherUsingExistingQueue() {
	// this assumes that the testQueue was created at the start of the test suite
	_, err := queue.NewPublisher(context.Background(), suite.QueueName, suite.publisherAuthOption)
	if suite.NoError(err) {
		// make sure that queue exists
		ns := suite.GetNewNamespace()
		tm := ns.NewQueueManager()
		_, err := tm.Get(context.Background(), suite.QueueName)
		require.NoError(suite.T(), err)
	}
}

// TestCreatePublisherWithDeadLetterForwardUsingNewQueue tests the creation of a publisher for a new queue
func (suite *serviceBusQueueSuite) TestCreatePublisherWithDeadLetterForwardUsingNewQueue() {
	testContext := suite.T()
	suite.Parallel()
	queueName := "newQueue" + suite.TagID
	_, err := queue.NewPublisher(context.Background(), queueName, suite.publisherAuthOption, publisher.WithForwardDeadLetteredMessagesTo(queueName+"DLQ", 1000))
	if suite.NoError(err) {
		// make sure that queue exists
		ns := suite.GetNewNamespace()
		tm := ns.NewQueueManager()
		_, err := tm.Get(context.Background(), queueName)
		require.NoError(testContext, err)

		// delete new queue
		err = tm.Delete(context.Background(), queueName)
		require.NoError(testContext, err)
	}
}

// TestPublishAfterIdle tests the creation of a publisher for an existing queue and a connection string
func (suite *serviceBusQueueSuite) TestPublishAfterIdle() {
	suite.T().Parallel()
	type idlenessTest struct {
		queueName string
		sleepTime time.Duration
	}

	tests := []idlenessTest{
		{queueName: "idle6min", sleepTime: 5 * time.Minute},
	}
	for _, idleTestCase := range tests {
		tc := idleTestCase
		suite.T().Run(suite.T().Name()+tc.queueName, func(test *testing.T) {
			test.Parallel()
			err := testQueueIdleness(suite.publisherAuthOption, tc.queueName, tc.sleepTime)
			require.NoError(test, err)
		})
	}
}

// TestPublishAfterIdle tests the creation of a publisher for an existing queue and a connection string
func (suite *serviceBusQueueSuite) TestSoakPub() {
	suite.T().Parallel()
	type idlenessTest struct {
		queueName string
		sleepTime time.Duration
	}

	tests := []idlenessTest{
		{queueName: "soakinterval2sec", sleepTime: 2 * time.Second},
		{queueName: "soakinterval30sec", sleepTime: 30 * time.Second},
		{queueName: "soakinterval5min", sleepTime: 2 * time.Minute},
	}

	soakTime := 10 * time.Minute
	deadline, _ := context.WithTimeout(context.Background(), soakTime)

	for _, soak := range tests {
		tc := soak
		suite.T().Run(suite.T().Name()+tc.queueName, func(test *testing.T) {
			test.Parallel()
			err := testQueueSoak(deadline, suite.publisherAuthOption, tc.queueName, tc.sleepTime)
			require.NoError(suite.T(), err)
		})
	}
}

func testQueueSoak(ctx context.Context, authOptions publisher.ManagementOption, queueName string, idleTime time.Duration) error {
	p, err := queue.NewPublisher(context.Background(), queueName, authOptions)
	if err != nil {
		return err
	}
	ok := true
	// stop on deadline
	go func() {
		<-ctx.Done()
		ok = false
	}()

	iteration := 0
	for ok {
		iteration++
		err := p.Publish(context.TODO(), &testEvent{
			ID:    iteration,
			Key:   "key",
			Value: "value",
		})
		if err != nil {
			return err
		}
		fmt.Println("[", queueName, "] published event ", iteration)
		select {
		case <-time.After(idleTime):
		case <-ctx.Done():
			continue
		}
	}
	return p.Close(context.TODO())
}

func testQueueIdleness(authOptions publisher.ManagementOption, queueName string, idleTime time.Duration) error {
	p, err := queue.NewPublisher(context.Background(), queueName, authOptions)
	if err != nil {
		return err
	}
	p.Publish(context.TODO(), &testEvent{
		ID:    1,
		Key:   "key1",
		Value: "value1",
	})
	time.Sleep(idleTime)
	return p.Publish(context.TODO(), &testEvent{
		ID:    2,
		Key:   "key2",
		Value: "value2",
	})
}
