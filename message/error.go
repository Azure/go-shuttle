package message

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/devigned/tab"
)

// Error is a wrapper around Abandon() that allows to trace the error before abandoning the message
func Error(e error) Handler {
	return &errorHandler{err: e}
}

type errorHandler struct {
	err error
}

func (a *errorHandler) Do(ctx context.Context, _ Handler, msg *servicebus.Message) Handler {
	ctx, span := startSpanFromMessageAndContext(ctx, "go-shuttle.errorHandler.Do", msg)
	defer span.End()

	if a.err != nil {
		tab.For(ctx).Debug(a.err.Error())
	}

	return Abandon()
}
