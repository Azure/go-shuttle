package handlers

import (
	"context"
	"fmt"

	servicebus "github.com/Azure/azure-service-bus-go"
)

var _ servicebus.Handler = (*concurrent)(nil)

// Concurrent is a servicebus handler starts concurrent workers processing messages
// maxConcurrency configures the maximum of concurrent workers started by the handler
type concurrent struct {
	next              servicebus.Handler
	concurrencyTokens chan bool
}

func NewConcurrent(maxConcurrency int, next servicebus.Handler) servicebus.Handler {
	if maxConcurrency < 1 {
		panic(fmt.Sprintf("maxConcurrency must be >= 1, but was %d", maxConcurrency))
	}
	if next == nil {
		panic(NextHandlerNilError.Error())
	}
	return &concurrent{
		next:              next,
		concurrencyTokens: make(chan bool, maxConcurrency),
	}
}

func (c *concurrent) Handle(ctx context.Context, msg *servicebus.Message) error {
	c.concurrencyTokens <- true
	go func() {
		defer func() { <-c.concurrencyTokens }()
		c.next.Handle(ctx, msg)
	}()
	// no error to return, the error are handled per message and don't affect the pipeline
	return nil
}
