package message

import (
	"context"
	"fmt"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
)

const renewLockInterval = 20 * time.Second

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

func (r *retryLaterHandler) Do(ctx context.Context, orig Handler, message *servicebus.Message, sub *servicebus.Subscription) Handler {
	select {
	case <-ctx.Done():
		return Error(fmt.Errorf("aborting retrylater for message. abandoning meddage %s: %w", message.ID, ctx.Err()))
	case <-time.After(renewLockInterval):
		err := sub.RenewLocks(ctx, message)
		if err != nil {
			return Error(fmt.Errorf("error renew locks, aborting retrylater for message. abandoning message %s: error: %s", message.ID, err))
		}
		return orig
	case <-time.After(r.retryAfter):
		err := sub.RenewLocks(ctx, message)
		if err != nil {
			return Error(fmt.Errorf("error renew locks, aborting retrylater for message. abandoning message %s: error: %s", message.ID, err))
		}
		return orig
	}
}
