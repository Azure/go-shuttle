package integration

import (
	"context"
	"sync"

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
			_, err := publisher.New(topicName, suite.publisherAuthOption)
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
