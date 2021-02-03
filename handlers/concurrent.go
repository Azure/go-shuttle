package handlers

import (
	"context"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/message"
	"github.com/devigned/tab"
	_ "go.uber.org/automaxprocs"
)

var _ servicebus.Handler = (*Concurrent)(nil)

// Concurrent is a servicebus handler starts concurrent workers processing messages
// maxConcurrency configures the maximum of concurrent workers started by the handler
type Concurrent struct {
	next              message.Handler
	concurrencyTokens chan bool
}

func NewConcurrent(maxConcurrency int, next message.Handler) *Concurrent {
	return &Concurrent{
		next:              next,
		concurrencyTokens: make(chan bool, maxConcurrency),
	}
}

func (c *Concurrent) Handle(ctx context.Context, msg *servicebus.Message) error {
	c.concurrencyTokens <- true
	go func() {
		defer func() { <-c.concurrencyTokens }()
		c.handleMessage(ctx, msg)
	}()
	// no error to return, the error are handled per message
	return nil
}

func hasDeadline(msg *servicebus.Message) bool {
	return msg.SystemProperties != nil &&
		msg.SystemProperties.EnqueuedTime != nil &&
		msg.TTL != nil &&
		*msg.TTL > 0*time.Second
}

func (c *Concurrent) handleMessage(ctx context.Context, msg *servicebus.Message) error {
	ctx, s := tab.StartSpan(ctx, "go-shuttle.Listener.HandlerFunc")
	defer s.End()
	currentHandler := c.next
	for !message.IsDone(currentHandler) {
		currentHandler = currentHandler.Do(ctx, c.next, msg)
		// handle nil as a Completion!
		if currentHandler == nil {
			currentHandler = message.Complete()
		}
	}
	return nil
}
