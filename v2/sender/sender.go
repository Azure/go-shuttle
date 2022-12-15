package sender

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

type MessageBody any

type SendHandlerFunction func(ctx context.Context, msg *azservicebus.Message)

type CustomSender interface {
	SendMessage(ctx context.Context, mb MessageBody, options *azservicebus.SendMessageOptions, middleware SendHandlerFunction) error
}

type DefaultSender struct {
	sender     *azservicebus.Sender
	marshaller Marshaller
}

var _ CustomSender = &DefaultSender{}

func NewDefaultSender(sender *azservicebus.Sender, marshaller Marshaller) *DefaultSender {
	return &DefaultSender{sender: sender, marshaller: marshaller}
}

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
