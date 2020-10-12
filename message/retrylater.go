package message

import (
	"context"
	"fmt"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
)

// RetryLater waits for the given duration before retrying the processing of the message.
// This happens in memory and does not impact servicebus message max retry limit
func RetryLater(retryAfter time.Duration) Handler {
	return &retryLaterHandler{
		retryAfter: retryAfter,
	}
}

type retryLaterHandler struct {
	retryAfter time.Duration
}

func (r *retryLaterHandler) Do(ctx context.Context, orig Handler, message *servicebus.Message) Handler {
	select {
	case <-ctx.Done():
		return Error(fmt.Errorf("aborting retrylater for message. abandoning meddage %s: %w", message.ID, ctx.Err()))
	case <-time.After(r.retryAfter):
		return orig
	}
}
