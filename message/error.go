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

func (h *errorHandler) Do(ctx context.Context, _ Handler, msg *servicebus.Message) Handler {
	ctx, span := startSpanFromMessageAndContext(ctx, "go-shuttle.errorHandler.Do", msg)
	defer span.End()
	span.AddAttributes(
		tab.BoolAttribute("error", true),
		tab.StringAttribute("level", "error"),
		tab.StringAttribute("error", h.err.Error()))

	return Abandon()
}

func IsError(h Handler) bool {
	_, ok := h.(*errorHandler)
	return ok
}
