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
	go func() {
		select {
		//TODO this can go past lock duration pretty easily
		//Ideally we'd also timeout at dequeue time + lockdurtaion - 1 second but don't have access to dequeue time
		//Maybe we can use context.WithTimeout when we recieve the message?
		case <-ctx.Done():
			//listener should cancel before lock duration which is the longest we can ask for retry later
			fmt.Printf("messsage context canceled\n")
			return
		case <-time.After(r.retryAfter):
			fmt.Printf("Abandoning messsage %v\n", time.Now())
			Abandon().Do(ctx, orig, message)
		}
	}()
	return done() //if we stick with abanon for retry we don't need to pass and return handlers
}
