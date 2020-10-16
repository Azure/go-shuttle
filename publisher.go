package shuttle

import (
	"github.com/Azure/go-shuttle/publisher"
)

// NewPublisher creates a new service bus publisher
// deprecated: use publisher.New(topicName string, opts ...publisher.ManagementOption)
func NewPublisher(topicName string, opts ...publisher.ManagementOption) (*publisher.Publisher, error) {
	return publisher.New(topicName, opts...)
}
