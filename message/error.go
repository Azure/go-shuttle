package message

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
)

// Error is a wrapper around Abandon() that allows to trace the error before abandoning the message
func Error(e error) Handler {
	return &errorHandler{err: e}
}

type errorHandler struct {
	err error
}

func (a *errorHandler) Do(_ context.Context, _ Handler, _ *servicebus.Message) Handler {
	// can trace the error before abandoning once we settle on a tracing system that can be passed via context
	return Abandon()
}
