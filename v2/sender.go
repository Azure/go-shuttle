package v2

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// MessageBody is a type to represent that an input message body can be of any type
type MessageBody any

// SendOption is a function that is used to modify the ServiceBus message before sending
type SendOption func(ctx context.Context, msg *azservicebus.Message)

// CustomSender is a wrapper around the ServiceBus sender that allows for users to introduce middleware to modify the ServiceBus message before it's sent
type CustomSender interface {
	SendMessage(ctx context.Context, mb MessageBody, options SendOption) error
}

// SBSender is satisfied by *azservicebus.Sender
type SBSender interface {
	SendMessage(ctx context.Context, message *azservicebus.Message, options *azservicebus.SendMessageOptions) error
}

// Sender contains an SBSender used to send the message to the ServiceBus queue and a Marshaller used to marshal any struct into a ServiceBus message
type Sender struct {
	sbSender SBSender
	options  SenderOptions
}

type SenderOptions struct {
	marshaller Marshaller
}

var _ CustomSender = &Sender{}

// NewSender takes in a Sender and a Marshaller to create a new object that can send messages to the ServiceBus queue
func NewSender(sender SBSender, options SenderOptions) *Sender {
	return &Sender{sbSender: sender, options: options}
}

// SendMessage marshals the input struct, runs middleware to modify the returned ServiceBus message, and uses the Sender to send the message to the ServiceBus queue
func (d *Sender) SendMessage(ctx context.Context, mb MessageBody, options SendOption) error {
	// uses a marshaller to marshal the message into a service bus message
	msg, err := d.options.Marshaller().Marshal(mb)
	if err != nil {
		return fmt.Errorf("failed to marshal original struct into ServiceBus message: %s", err)
	}

	// run user-provided middleware
	if options != nil {
		options(ctx, msg)
	}

	err = d.sbSender.SendMessage(ctx, msg, &azservicebus.SendMessageOptions{}) // sendMessageOptions currently does nothing
	if err != nil {
		return fmt.Errorf("failed to send message: %s", err)
	}

	return nil
}

func (s SenderOptions) Marshaller() Marshaller {
	return s.marshaller
}

// SetTypeHandler sets the ServiceBus message's type to the original struct's type
func SetTypeHandler(mb MessageBody, options SendOption) SendOption {
	return func(ctx context.Context, msg *azservicebus.Message) {
		msgType := GetMessageType(mb)
		msg.ContentType = &msgType
		options(ctx, msg)
	}
}

// SetMessageIdHandler sets the ServiceBus message's ID to a user-specified value
func SetMessageIdHandler(messageId *string, options SendOption) SendOption {
	return func(ctx context.Context, msg *azservicebus.Message) {
		msg.MessageID = messageId
		options(ctx, msg)
	}
}

func MarshalHandler(marshaller Marshaller, mb MessageBody, options SendOption) SendOption {
	return func(ctx context.Context, msg *azservicebus.Message) {
		marshalledMsg, err := marshaller.Marshal(mb)
		if err != nil {
			log(ctx, "failed to marshal message: %s", err)
		}
		options(ctx, marshalledMsg)
	}
}

func GetMessageType(mb MessageBody) string {
	var msgType string
	vo := reflect.ValueOf(mb)
	if vo.Kind() == reflect.Ptr {
		msgType = reflect.Indirect(vo).Type().Name()
	} else {
		msgType = vo.Type().Name()
	}

	return msgType
}
