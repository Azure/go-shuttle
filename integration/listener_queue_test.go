// +build integration

package integration

import (
	"context"
	"time"

	"github.com/Azure/go-shuttle/queue"
)

// TestCreateNewListenerFromConnectionString tests the creation of a listener with a connection string
func (suite *serviceBusQueueSuite) TestCreateNewQueueListener() {
	_, err := queue.NewListener(suite.listenerAuthOption)
	suite.NoError(err)
}

func (suite *serviceBusQueueSuite) TestListenWithDefault() {
	listener, err := queue.NewListener(suite.listenerAuthOption)
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
	}
}
