package concurrent

import (
	"context"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/message"
	"github.com/Azure/go-shuttle/peeklock"
	"github.com/devigned/tab"
)

type Handler struct {
	messageHandler  message.Handler
	messages        chan *servicebus.Message
	lockRenewer     peeklock.LockRenewer
	renewalInterval *time.Duration
}

func NewHandler(ctx context.Context, r peeklock.LockRenewer, renewalInterval *time.Duration, handler message.Handler) servicebus.Handler {
	h := &Handler{
		renewalInterval: renewalInterval,
		lockRenewer:     r,
		messageHandler:  handler,
		messages:        make(chan *servicebus.Message),
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
	if hasDeadline(msg) {
		deadline := msg.SystemProperties.EnqueuedTime.Add(*msg.TTL)
		msgCtx, _ = context.WithDeadline(ctx, deadline)
	}
	return msgCtx
}

func hasDeadline(msg *servicebus.Message) bool {
	return msg.SystemProperties != nil &&
		msg.SystemProperties.EnqueuedTime != nil &&
		msg.TTL != nil &&
		*msg.TTL > 0*time.Second
}

func (h *Handler) handleMessage(ctx context.Context, msg *servicebus.Message) error {
	ctx, s := tab.StartSpan(ctx, "go-shuttle.Listener.HandlerFunc")
	defer s.End()
	// TODO: find a way to extract the lockRenewal concept completely out of the Handler. maybe via message channel
	if h.renewalInterval != nil {
		renewer := peeklock.RenewPeriodically(ctx, *h.renewalInterval, h.lockRenewer, msg)
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
