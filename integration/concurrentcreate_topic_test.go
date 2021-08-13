// +build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Azure/go-shuttle/topic"
	"github.com/Azure/go-shuttle/topic/listener"
	"github.com/stretchr/testify/require"

	"github.com/Azure/go-shuttle/message"
)

func (suite *serviceBusTopicSuite) TestCreatePublisherConcurrently() {
	t := suite.T()
	topicName := "concpublisher" + suite.TagID
	w := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		w.Add(1)
		go func(it int) {
			defer w.Done()
			_, err := topic.NewPublisher(context.Background(), topicName, suite.publisherAuthOption)
			require.NoError(t, err, "failed on iteration %d", it)
		}(i)
	}

	w.Wait()
	// make sure that topic exists
	ns := suite.GetNewNamespace()
	tm := ns.NewTopicManager()
	_, err := tm.Get(context.Background(), topicName)
	require.NoError(t, err)

	// delete new topic
	err = tm.Delete(context.Background(), topicName)
	require.NoError(t, err)
}

func (suite *serviceBusTopicSuite) TestCreateListenersConcurrently() {
	t := suite.T()
	topicName := "conclistener" + suite.TagID
	_, err := topic.NewPublisher(context.Background(), topicName, suite.publisherAuthOption)
	require.NoError(t, err)
	w := sync.WaitGroup{}
	deadlineContext, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()
	for i := 0; i < 5; i++ {
		l, err := topic.NewListener(suite.listenerAuthOption, listener.WithSubscriptionName("concurrentListener"))
		require.NoError(t, err)
		w.Add(1)
		go func(it int) {
			defer w.Done()
			h := message.HandleFunc(func(ctx context.Context, msg *message.Message) message.Handler {
				return message.Complete()
			})
			err = l.Listen(deadlineContext, h, topicName)
			if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
				t.Logf("ERROR! Listener %d failed to start: %s", it, err)
				// one of the concurrent create failed. we can cancel the whole context
				cancel()
				require.NoError(t, err)
			}
		}(i)
	}
	w.Wait()
	// delete the topic
	ns := suite.GetNewNamespace()
	tm := ns.NewTopicManager()
	err = tm.Delete(context.Background(), topicName)
	require.NoError(t, err)
}
