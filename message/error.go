package message

import (
	"context"
	"errors"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-amqp"
	"github.com/Azure/go-shuttle/tracing"
)

// Error is a wrapper around Abandon() that allows to trace the error before abandoning the message
func Error(e error) Handler {
	return &errorHandler{err: e}
}

type errorHandler struct {
	err error
}

func (h *errorHandler) Do(ctx context.Context, _ Handler, msg *servicebus.Message) Handler {
	ctx, span := tracing.StartSpanFromMessageAndContext(ctx, "go-shuttle.errorHandler.Do", msg)
	defer span.End()
	span.Logger().Error(h.err)
	var amqpErr *amqp.Error
	if errors.As(h.err, &amqpErr) {
		span.Logger().Info("error was amqp error. will not attempt to abandon the message")
		// the processing will terminate and the lock on the message will eventually be released after
		// the message lock expires on the broker side
		return done()
	}
	return Abandon()
}

func IsError(h Handler) bool {
	_, ok := h.(*errorHandler)
	return ok
}
