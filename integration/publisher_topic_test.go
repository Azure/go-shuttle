// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/go-shuttle/topic"
	"github.com/Azure/go-shuttle/topic/publisher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreatePublisherWithNewTopic tests the creation of a publisher for a new topic
func (suite *serviceBusTopicSuite) TestCreatePublisherUsingNewTopic() {
	t := suite.T()
	t.Parallel()
	topicName := "newTopic" + suite.TagID
	_, err := topic.NewPublisher(context.Background(), topicName, suite.publisherAuthOption)

	if assert.NoError(t, err) {
		// make sure that topic exists
		ns := suite.GetNewNamespace()
		tm := ns.NewTopicManager()
		_, err := tm.Get(context.Background(), topicName)
		require.NoError(t, err)

		// delete new topic
		err = tm.Delete(context.Background(), topicName)
		require.NoError(t, err)
	}
}

// TestCreatePublisherWithExistingTopic tests the creation of a publisher for an existing topic and a connection string
func (suite *serviceBusTopicSuite) TestCreatePublisherUsingExistingTopic() {
	t := suite.T()
	t.Parallel()
	// this assumes that the testTopic was created at the start of the test suite
	_, err := topic.NewPublisher(context.Background(), suite.TopicName, suite.publisherAuthOption)
	if assert.NoError(t, err) {
		// make sure that topic exists
		ns := suite.GetNewNamespace()
		tm := ns.NewTopicManager()
		_, err := tm.Get(context.Background(), suite.TopicName)
		require.NoError(t, err)
	}
}

// TestPublishAfterIdle tests the creation of a publisher for an existing topic and a connection string
func (suite *serviceBusTopicSuite) TestPublishAfterIdle() {
	t := suite.T()
	t.Parallel()
	type idlenessTest struct {
		topicName string
		sleepTime time.Duration
	}

	tests := []idlenessTest{
		{topicName: "idle1min", sleepTime: 1 * time.Minute},
		{topicName: "idle3min", sleepTime: 3 * time.Minute},
		{topicName: "idle6min", sleepTime: 6 * time.Minute},
	}
	for _, idleTestCase := range tests {
		tc := idleTestCase
		t.Run(t.Name()+tc.topicName, func(tt *testing.T) {
			err := testIdleness(suite.publisherAuthOption, tc.topicName, tc.sleepTime)
			require.NoError(tt, err)
		})
	}
}

// TestPublishAfterIdle tests the creation of a publisher for an existing topic and a connection string
func (suite *serviceBusTopicSuite) TestSoakPub() {
	t := suite.T()
	t.Parallel()
	type idlenessTest struct {
		topicName string
		sleepTime time.Duration
	}

	tests := []idlenessTest{
		{topicName: "soakinterval2sec", sleepTime: 2 * time.Second},
		{topicName: "soakinterval30sec", sleepTime: 30 * time.Second},
		{topicName: "soakinterval2min", sleepTime: 2 * time.Minute},
	}

	soakTime := 10 * time.Minute
	deadline, _ := context.WithTimeout(context.Background(), soakTime)

	for _, soak := range tests {
		tc := soak
		t.Run(t.Name()+tc.topicName, func(tt *testing.T) {
			tt.Parallel()
			err := testTopicSoak(deadline, suite.publisherAuthOption, tc.topicName, tc.sleepTime)
			require.NoError(tt, err)
		})
	}
}

func testTopicSoak(ctx context.Context, authOptions publisher.ManagementOption, topicName string, idleTime time.Duration) error {
	p, err := topic.NewPublisher(context.Background(), topicName, authOptions)
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
		select {
		case <-time.After(idleTime):
		case <-ctx.Done():
			ok = false
			continue
		}
	}
	return p.Close(context.TODO())
}

func testIdleness(authOptions publisher.ManagementOption, topicName string, idleTime time.Duration) error {
	p, err := topic.NewPublisher(context.Background(), topicName, authOptions)
	if err != nil {
		return err
	}
	if err = p.Publish(context.TODO(), &testEvent{
		ID:    1,
		Key:   "key1",
		Value: "value1",
	}); err != nil {
		return err
	}
	time.Sleep(idleTime)
	return p.Publish(context.TODO(), &testEvent{
		ID:    2,
		Key:   "key2",
		Value: "value2",
	})
}
