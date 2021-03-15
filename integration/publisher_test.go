package integration

import (
	"context"

	"github.com/Azure/go-shuttle/publisher"
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
