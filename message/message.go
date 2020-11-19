package message

import (
	"fmt"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
)

// Message is the wrapping type of service bus message with a type
type Message struct {
	msg         *servicebus.Message
	messageType string
}

// New creates a message, validating type first
func New(msg *servicebus.Message) (*Message, error) {
	messageType, ok := msg.UserProperties["type"]
	if !ok {
		return nil, fmt.Errorf("message did not include a \"type\" in UserProperties")
	}
	return &Message{msg, messageType.(string)}, nil
}

// Message returns the message as received by the SDK
func (m *Message) Message() *servicebus.Message {
	return m.msg
}

// Type returns the message type as a string, passed on the message UserProperties
func (m *Message) Type() string {
	return m.messageType
}

// Data returns the message data as a string. Can be unmarshalled to the original struct
func (m *Message) Data() string {
	return string(m.msg.Data)
}

// Complete will notify Azure Service Bus that the message was successfully handled and should be deleted from the queue
func (m *Message) Complete() Handler {
	return Complete()
}

// Abandon will notify Azure Service Bus the message failed but should be re-queued for delivery.
func (m *Message) Abandon() Handler {
	return Abandon()
}

// Error is a wrapper around Abandon() that allows to trace the error before abandoning the message
func (m *Message) Error(err error) Handler {
	return Error(err)
}

// RetryLater waits for the given duration before retrying the processing of the message.
// This happens in memory and does not impact servicebus message max retry limit
func (m *Message) RetryLater(retryAfter time.Duration) Handler {
	return RetryLater(retryAfter)
}
