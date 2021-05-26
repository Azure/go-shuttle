// +build integration

package integration

import (
	"context"
	"fmt"
	"github.com/Azure/go-shuttle/publisher/queue"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestCreatePublisherWithNewTopic tests the creation of a publisher for a new topic
func (suite *serviceBusQueueSuite) TestCreatePublisherUsingNewQueue() {
	suite.T().Parallel()
	queueName := "newQueue" + suite.TagID
	_, err := queue.New(context.Background(), queueName, suite.publisherAuthOption)
	if suite.NoError(err) {
		// make sure that topic exists
		ns := suite.GetNewNamespace()
		tm := ns.NewQueueManager()
		_, err := tm.Get(context.Background(), queueName)
		suite.NoError(err)

		// delete new topic
		err = tm.Delete(context.Background(), queueName)
		suite.NoError(err)
	}
}

// TestCreatePublisherWithExistingQueue tests the creation of a publisher for an existing queue and a connection string
func (suite *serviceBusQueueSuite) TestCreatePublisherUsingExistingQueue() {
	// this assumes that the testTopic was created at the start of the test suite
	_, err := queue.New(context.Background(), suite.QueueName, suite.publisherAuthOption)
	if suite.NoError(err) {
		// make sure that topic exists
		ns := suite.GetNewNamespace()
		tm := ns.NewQueueManager()
		_, err := tm.Get(context.Background(), suite.QueueName)
		suite.NoError(err)
	}
}

// TestPublishAfterIdle tests the creation of a publisher for an existing topic and a connection string
func (suite *serviceBusQueueSuite) TestPublishAfterIdle() {
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
			assert.NoError(test, err)
		})
	}
}

// TestPublishAfterIdle tests the creation of a publisher for an existing topic and a connection string
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
			if !assert.NoError(test, err) {
				suite.FailNow(err.Error())
			}
		})
	}
}

func testQueueSoak(ctx context.Context, authOptions queue.ManagementOption, queueName string, idleTime time.Duration) error {
	p, err := queue.New(context.Background(), queueName, authOptions)
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

func testQueueIdleness(authOptions queue.ManagementOption, queueName string, idleTime time.Duration) error {
	p, err := queue.New(context.Background(), queueName, authOptions)
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
