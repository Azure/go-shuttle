package handlers_test

import (
	"context"
	"testing"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/handlers"
	"github.com/stretchr/testify/assert"
)

func Test_CancelContextWhenDoneWithMessage(t *testing.T) {
	next := &FakeHandler{}
	handler := handlers.NewDeadlineContext(next)
	ctx := context.Background()
	msg := &servicebus.Message{}
	err := handler.Handle(ctx, msg)
	assert.Nil(t, err)
	assert.Equal(t, context.Canceled, next.Ctx.Err())
}

func Test_DownstreamContextDeadlinesWhenDeadlineExceeded(t *testing.T) {
	next := &FakeHandler{}
	enqueuedTime := time.Now()
	ttl := 1 * time.Microsecond
	next.HandleFunc = func(ctx context.Context, msg *servicebus.Message) error {
		time.Sleep(2 * ttl) // ensure we pass the deadline
		return nil
	}
	handler := handlers.NewDeadlineContext(next)
	ctx := context.Background()
	ttlMessage := &servicebus.Message{
		SystemProperties: &servicebus.SystemProperties{
			EnqueuedTime: &enqueuedTime,
		},
		TTL: &ttl,
	}
	err := handler.Handle(ctx, ttlMessage)
	assert.Nil(t, err)
	assert.Equal(t, context.DeadlineExceeded, next.Ctx.Err())
}

func Test_DownstreamContextCanceledWhenDoneBeforeDeadlineExceeded(t *testing.T) {
	next := &FakeHandler{}
	enqueuedTime := time.Now()
	ttl := 100 * time.Millisecond
	next.HandleFunc = func(ctx context.Context, msg *servicebus.Message) error {
		return nil
	}
	handler := handlers.NewDeadlineContext(next)
	ctx := context.Background()
	ttlMessage := &servicebus.Message{
		SystemProperties: &servicebus.SystemProperties{
			EnqueuedTime: &enqueuedTime,
		},
		TTL: &ttl,
	}
	err := handler.Handle(ctx, ttlMessage)
	assert.Nil(t, err)
	assert.Equal(t, context.Canceled, next.Ctx.Err())
}
