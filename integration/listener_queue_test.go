// +build integration

package integration

import (
	"context"
	"github.com/Azure/go-shuttle/listener/queue"
	"time"
)

// TestCreateNewListenerFromConnectionString tests the creation of a listener with a connection string
func (suite *serviceBusQueueSuite) TestCreateNewQueueListener() {
	_, err := queue.New(suite.listenerAuthOption)
	suite.NoError(err)
}

func (suite *serviceBusQueueSuite) TestListenWithDefault() {
	listener, err := queue.New(suite.listenerAuthOption)
	if suite.NoError(err) {
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		go func() {
			time.Sleep(10 * time.Second)
			cancel()
		}()
		err := listener.Listen(
			ctx,
			createTestHandler(),
			suite.QueueName,
		)
		suite.True(contextCanceledError(err), "listener listen function failed", err.Error())
		suite.EqualValues(listener.Queue().Name, "default")
	}
}
