// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/go-shuttle/publisher"
	"github.com/stretchr/testify/assert"
)

// TestCreatePublisherWithNewTopic tests the creation of a publisher for a new topic
func (suite *serviceBusSuite) TestCreatePublisherUsingNewTopic() {
	suite.T().Parallel()
	topicName := "newTopic" + suite.TagID
	_, err := publisher.New(context.Background(), topicName, suite.publisherAuthOption)
	if suite.NoError(err) {
		// make sure that topic exists
		ns := suite.GetNewNamespace()
		tm := ns.NewTopicManager()
		_, err := tm.Get(context.Background(), topicName)
		suite.NoError(err)

		// delete new topic
		err = tm.Delete(context.Background(), topicName)
		suite.NoError(err)
	}
}

// TestCreatePublisherWithExistingTopic tests the creation of a publisher for an existing topic and a connection string
func (suite *serviceBusSuite) TestCreatePublisherUsingExistingTopic() {
	// this assumes that the testTopic was created at the start of the test suite
	_, err := publisher.New(context.Background(), suite.TopicName, suite.publisherAuthOption)
	if suite.NoError(err) {
		// make sure that topic exists
		ns := suite.GetNewNamespace()
		tm := ns.NewTopicManager()
		_, err := tm.Get(context.Background(), suite.TopicName)
		suite.NoError(err)
	}
}

// TestPublishAfterIdle tests the creation of a publisher for an existing topic and a connection string
func (suite *serviceBusSuite) TestPublishAfterIdle() {
	type idlenessTest struct {
		topicName string
		sleepTime time.Duration
	}

	tests := []idlenessTest{
		{topicName: "idle6min", sleepTime: 5 * time.Minute},
	}
	for _, idleTestCase := range tests {
		tc := idleTestCase
		suite.T().Run(suite.T().Name()+tc.topicName, func(test *testing.T) {
			test.Parallel()
			err := testIdleness(suite.publisherAuthOption, tc.topicName, tc.sleepTime)
			assert.NoError(test, err)
		})
	}
}

// TestPublishAfterIdle tests the creation of a publisher for an existing topic and a connection string
func (suite *serviceBusSuite) TestSoakPub() {
	suite.T().Parallel()
	type idlenessTest struct {
		topicName string
		sleepTime time.Duration
	}

	tests := []idlenessTest{
		{topicName: "soakinterval2sec", sleepTime: 2 * time.Second},
		{topicName: "soakinterval30sec", sleepTime: 30 * time.Second},
		{topicName: "soakinterval5min", sleepTime: 2 * time.Minute},
	}

	soakTime := 10 * time.Minute
	deadline, _ := context.WithTimeout(context.Background(), soakTime)

	for _, soak := range tests {
		tc := soak
		suite.T().Run(suite.T().Name()+tc.topicName, func(test *testing.T) {
			test.Parallel()
			err := testSoak(deadline, suite.publisherAuthOption, tc.topicName, tc.sleepTime)
			assert.NoError(test, err)
			suite.FailNow(err.Error())
		})
	}
}

func testSoak(ctx context.Context, authOptions publisher.ManagementOption, topicName string, idleTime time.Duration) error {
	p, err := publisher.New(context.Background(), topicName, authOptions)
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
		fmt.Println("[", topicName, "] published event ", iteration)
		time.Sleep(idleTime)
	}
	return p.Close(context.TODO())
}

func testIdleness(authOptions publisher.ManagementOption, topicName string, idleTime time.Duration) error {
	p, err := publisher.New(context.Background(), topicName, authOptions)
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
