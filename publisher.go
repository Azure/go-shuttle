package pubsub

import (
	"github.com/keikumata/azure-pub-sub/publisher"
)

// NewPublisher creates a new service bus publisher
// deprecated: use publisher.New(topicName string, opts ...publisher.ManagementOption)
func NewPublisher(topicName string, opts ...publisher.ManagementOption) (*publisher.Publisher, error) {
	return publisher.New(topicName, opts...)
}
