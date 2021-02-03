package handlers

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
)

var _ servicebus.Handler = (*Concurrent)(nil)

// Concurrent is a servicebus handler starts concurrent workers processing messages
// maxConcurrency configures the maximum of concurrent workers started by the handler
type Concurrent struct {
	next              servicebus.Handler
	concurrencyTokens chan bool
}

func NewConcurrent(maxConcurrency int, next servicebus.Handler) *Concurrent {
	return &Concurrent{
		next:              next,
		concurrencyTokens: make(chan bool, maxConcurrency),
	}
}

func (c *Concurrent) Handle(ctx context.Context, msg *servicebus.Message) error {
	if c.next == nil {
		return NextHandlerNilError
	}
	c.concurrencyTokens <- true
	go func() {
		defer func() { <-c.concurrencyTokens }()
		c.next.Handle(ctx, msg)
		// how do we cancel the message context here?
	}()
	// no error to return, the error are handled per message and don't affect the pipeline
	return nil
}
