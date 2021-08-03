package topic

import (
"context"
"github.com/Azure/go-shuttle/topic/listener"
"github.com/Azure/go-shuttle/topic/publisher"
)

func NewListener(opts ...listener.ManagementOption) (*listener.Listener, error) {
	return listener.New(opts...)
}

func NewPublisher(ctx context.Context, queueName string, opts ...publisher.ManagementOption) (*publisher.Publisher, error) {
	return publisher.New(ctx, queueName, opts...)
}
