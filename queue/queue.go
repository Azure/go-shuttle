package queue

import (
	"context"
	"github.com/Azure/go-shuttle/common/options/listeneropts"
	"github.com/Azure/go-shuttle/queue/listener"
	"github.com/Azure/go-shuttle/queue/publisher"
)

func NewListener(opts ...listeneropts.ManagementOption) (*listener.Listener, error) {
	return listener.New(opts...)
}

func NewPublisher(ctx context.Context, queueName string, opts ...publisher.ManagementOption) (*publisher.Publisher, error) {
	return publisher.New(ctx, queueName, opts...)
}
