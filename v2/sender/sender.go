package sender

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// MessageBody is a type to represent that an input message body can be of any type
type MessageBody any

// SendHandlerFunction is a function that is used to modify the ServiceBus message before sending
type SendHandlerFunction func(ctx context.Context, msg *azservicebus.Message)

// CustomSender is a wrapper around the ServiceBus sender that allows for users to introduce middleware to modify the ServiceBus message before it's sent
type CustomSender interface {
	SendMessage(ctx context.Context, mb MessageBody, options *azservicebus.SendMessageOptions, middleware SendHandlerFunction) error
}

// Sender is an interface that allows users to substitute their own senders for the default ServiceBus sender when making a CustomSender
type Sender interface {
	SendMessage(ctx context.Context, message *azservicebus.Message, options *azservicebus.SendMessageOptions) error
}

// DefaultSender contains a Sender used to send the message to the ServiceBus queue and a Marshaller used to marshal any struct into a ServiceBus message
type DefaultSender struct {
	sender     Sender
	marshaller Marshaller
}

var _ CustomSender = &DefaultSender{}

// NewDefaultSender takes in a Sender and a Marshaller to create a new object that can send messages to the ServiceBus queue
func NewDefaultSender(sender Sender, marshaller Marshaller) *DefaultSender {
	return &DefaultSender{sender: sender, marshaller: marshaller}
}

// SendMessage marshals the input struct, runs middleware to modify the returned ServiceBus message, and uses the Sender to send the message to the ServiceBus queue
func (d *DefaultSender) SendMessage(ctx context.Context, mb MessageBody, options *azservicebus.SendMessageOptions, middleware SendHandlerFunction) error {
	// uses a marshaller to marshal the message into a service bus message
	msg, err := d.marshaller.Marshal(mb)
	if err != nil {
		return fmt.Errorf("failed to marshal original struct into ServiceBus message: %s", err)
	}

	// run user-provided middleware
	if middleware != nil {
		middleware(ctx, msg)
	}

	err = d.sender.SendMessage(ctx, msg, options)
	if err != nil {
		return fmt.Errorf("failed to send message: %s", err)
	}

	return nil
}

// SetTypeHandler sets the ServiceBus message's type to the original struct's type
func SetTypeHandler(mb MessageBody, handler SendHandlerFunction) SendHandlerFunction {
	return func(ctx context.Context, msg *azservicebus.Message) {
		var msgType string
		vo := reflect.ValueOf(mb)
		if vo.Kind() == reflect.Ptr {
			msgType = reflect.Indirect(vo).Type().Name()
		} else {
			msgType = vo.Type().Name()
		}
		msg.ContentType = &msgType

		handler(ctx, msg)
	}
}

// SetMessageIdHandler sets the ServiceBus message's ID to a user-specified value
func SetMessageIdHandler(messageId *string, handler SendHandlerFunction) SendHandlerFunction {
	return func(ctx context.Context, msg *azservicebus.Message) {
		msg.MessageID = messageId
		handler(ctx, msg)
	}
}
