package concurrent

import (
	"context"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/listener"
	"github.com/Azure/go-shuttle/message"
	"github.com/Azure/go-shuttle/peeklock"
	"github.com/devigned/tab"
)

type Handler struct {
	listener       *listener.Listener
	lockRenewer    peeklock.LockRenewer
	messageHandler message.Handler
	messages       chan *servicebus.Message
}

func NewHandler(ctx context.Context, l *listener.Listener, r peeklock.LockRenewer, handler message.Handler) servicebus.Handler {
	h := &Handler{
		listener:       l,
		lockRenewer:    r,
		messageHandler: handler,
		messages:       make(chan *servicebus.Message),
	}
	go h.handleMessages(ctx)
	return h
}

func (h *Handler) Handle(_ context.Context, msg *servicebus.Message) error {
	h.messages <- msg
	// servicebus handler errors interrupt the listener. A single message handling error should not stop the listener.
	// always return nil here
	return nil
}

func (h *Handler) handleMessages(ctx context.Context) {
	for msg := range h.messages {
		go h.handleMessage(messageContext(ctx, msg), msg)
	}
}

func messageContext(ctx context.Context, msg *servicebus.Message) context.Context {
	msgCtx, _ := context.WithCancel(ctx)
	if msg.SystemProperties != nil && msg.SystemProperties.EnqueuedTime != nil && msg.TTL != nil {
		msgCtx, _ = context.WithDeadline(ctx, msg.SystemProperties.EnqueuedTime.Add(*msg.TTL))
	}
	return msgCtx
}

func (h *Handler) handleMessage(ctx context.Context, msg *servicebus.Message) error {
	ctx, s := tab.StartSpan(ctx, "go-shuttle.Listener.HandlerFunc")
	defer s.End()
	if h.listener.LockRenewalInterval != nil {
		renewer := peeklock.RenewPeriodically(ctx, *h.listener.LockRenewalInterval, h.lockRenewer, msg)
		defer renewer.Stop()
	}
	currentHandler := h.messageHandler
	for !message.IsDone(currentHandler) {
		currentHandler = currentHandler.Do(ctx, h.messageHandler, msg)
		// handle nil as a Completion!
		if currentHandler == nil {
			currentHandler = message.Complete()
		}
	}
	return nil
}
