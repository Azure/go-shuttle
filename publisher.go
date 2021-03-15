package shuttle

import (
	"context"

	"github.com/Azure/go-shuttle/publisher"
)

// NewPublisher creates a new service bus publisher
// deprecated: use publisher.New(ctx context.Context, topicName string, opts ...publisher.ManagementOption)
// injects context.Background for top-level compatability
func NewPublisher(topicName string, opts ...publisher.ManagementOption) (*publisher.Publisher, error) {
	return publisher.New(context.Background(), topicName, opts...)
}

// NewPublisherWithContext creates a new service bus publisher with the provided ctx
// deprecated: use publisher.New(ctx context.Context, topicName string, opts ...publisher.ManagementOption)
func NewPublisherWithContext(ctx context.Context, topicName string, opts ...publisher.ManagementOption) (*publisher.Publisher, error) {
	return publisher.New(ctx, topicName, opts...)
}
