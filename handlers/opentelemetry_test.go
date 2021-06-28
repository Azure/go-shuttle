package handlers_test

import (
	"context"
	"testing"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/handlers"
	. "github.com/onsi/gomega"
)

type RecorderHandler struct {
	inCtx context.Context
	msg   *servicebus.Message
}

func (h *RecorderHandler) Handle(ctx context.Context, msg *servicebus.Message) error {
	h.inCtx = ctx
	msg = msg
	return nil
}

func TestNew(t *testing.T) {
	g := NewWithT(t)
	h := handlers.NewTracePropagator(&NoOpHandler{}, handlers.OpenCensus)
	err := h.Handle(context.TODO(), &servicebus.Message{})
	g.Expect(err).ToNot(HaveOccurred())
}
