package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/go-shuttle/listener"
	"github.com/Azure/go-shuttle/message"
	"github.com/Azure/go-shuttle/publisher"
	"github.com/stretchr/testify/assert"
)

func (suite *serviceBusSuite) TestCreatePublisherConcurrently() {
	suite.T().Parallel()
	topicName := "newTopic" + suite.TagID
	w := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		w.Add(1)
		go func(it int) {
			defer w.Done()
			_, err := publisher.New(context.Background(), topicName, suite.publisherAuthOption)
			if !assert.NoError(suite.T(), err, "failed on iteration %d", it) {
				suite.FailNow(err.Error())
			}
		}(i)
	}

	w.Wait()
	// make sure that topic exists
	ns := suite.GetNewNamespace()
	tm := ns.NewTopicManager()
	_, err := tm.Get(context.Background(), topicName)
	suite.NoError(err)

	// delete new topic
	err = tm.Delete(context.Background(), topicName)
	suite.NoError(err)
}

func (suite *serviceBusSuite) TestCreateListenersConcurrently() {
	suite.T().Parallel()
	topicName := "newTopic" + suite.TagID
	_, err := publisher.New(context.Background(), topicName, suite.publisherAuthOption)
	assert.NoError(suite.T(), err)
	w := sync.WaitGroup{}
	lctx, cancel := context.WithTimeout(context.TODO(), 20*time.Second)
	for i := 0; i < 5; i++ {
		l, err := listener.New(suite.listenerAuthOption, listener.WithSubscriptionName("concurrentListener"))
		assert.NoError(suite.T(), err)
		w.Add(1)
		go func(it int) {
			defer w.Done()
			h := message.HandleFunc(func(ctx context.Context, msg *message.Message) message.Handler {
				return message.Complete()
			})
			err = l.Listen(lctx, h, topicName)
			if !errors.Is(err, context.DeadlineExceeded) {
				fmt.Printf("ERROR! Listener %d failed to start: %s", i, err)
				assert.NoError(suite.T(), err)
				cancel()
			}
		}(i)
	}
	w.Wait()
	// delete the topic
	ns := suite.GetNewNamespace()
	tm := ns.NewTopicManager()
	err = tm.Delete(context.Background(), topicName)
	suite.NoError(err)
}
