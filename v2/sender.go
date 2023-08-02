package shuttle

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

const (
	msgTypeField       = "type"
	DefaultSendTimeout = 10 * time.Second
)

// MessageBody is a type to represent that an input message body can be of any type
type MessageBody any

// AzServiceBusSender is satisfied by *azservicebus.Sender
type AzServiceBusSender interface {
	SendMessage(ctx context.Context, message *azservicebus.Message, options *azservicebus.SendMessageOptions) error
	SendMessageBatch(ctx context.Context, batch *azservicebus.MessageBatch, options *azservicebus.SendMessageBatchOptions) error
	NewMessageBatch(ctx context.Context, options *azservicebus.MessageBatchOptions) (*azservicebus.MessageBatch, error)
}

// Sender contains an SBSender used to send the message to the ServiceBus queue and a Marshaller used to marshal any struct into a ServiceBus message
type Sender struct {
	sbSender AzServiceBusSender
	options  *SenderOptions
}

type SenderOptions struct {
	// Marshaller will be used to marshall the messageBody to the azservicebus.Message Body property
	// defaults to DefaultJSONMarshaller
	Marshaller Marshaller
	// EnableTracingPropagation automatically applies WithTracePropagation option on all message sent through this sender
	EnableTracingPropagation bool
	// SendTimeout is the default timeout for sending a message
	SendTimeout time.Duration
}

// NewSender takes in a Sender and a Marshaller to create a new object that can send messages to the ServiceBus queue
func NewSender(sender AzServiceBusSender, options *SenderOptions) *Sender {
	if options == nil {
		options = &SenderOptions{Marshaller: &DefaultJSONMarshaller{}}
	}
	if options.SendTimeout == 0 {
		options.SendTimeout = DefaultSendTimeout
	}
	return &Sender{sbSender: sender, options: options}
}

// SendMessage sends a payload on the bus.
// the MessageBody is marshalled and set as the message body.
func (d *Sender) SendMessage(ctx context.Context, mb MessageBody, options ...func(msg *azservicebus.Message) error) error {
	msg, err := d.ToServiceBusMessage(ctx, mb, options...)
	if err != nil {
		return err
	}
	if d.options.SendTimeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, d.options.SendTimeout)
		defer cancel()
	}
	if err := d.sbSender.SendMessage(ctx, msg, nil); err != nil { // sendMessageOptions currently does nothing
		return fmt.Errorf("failed to send message: %w", err)
	}
	return nil
}

// ToServiceBusMessage transform a MessageBody into an azservicebus.Message.
// It marshals the body using the sender's configured marshaller,
// and set the bytes as the message.Body.
// the sender's configured options are applied to the azservicebus.Message before
// returning it.
func (d *Sender) ToServiceBusMessage(
	ctx context.Context,
	mb MessageBody,
	options ...func(msg *azservicebus.Message) error) (*azservicebus.Message, error) {
	// uses a marshaller to marshal the message into a service bus message
	msg, err := d.options.Marshaller.Marshal(mb)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal original struct into ServiceBus message: %w", err)
	}
	msgType := getMessageType(mb)
	msg.ApplicationProperties = map[string]interface{}{msgTypeField: msgType}

	if d.options.EnableTracingPropagation {
		options = append(options, WithTracePropagation(ctx))
	}

	for _, option := range options {
		if err := option(msg); err != nil {
			return nil, fmt.Errorf("failed to run message options: %w", err)
		}
	}
	return msg, nil
}

// SendMessageBatch sends the array of azservicebus messages as a batch.
func (d *Sender) SendMessageBatch(ctx context.Context, messages []*azservicebus.Message) error {
	batch, err := d.sbSender.NewMessageBatch(ctx, &azservicebus.MessageBatchOptions{})
	if err != nil {
		return err
	}
	for _, msg := range messages {
		if err := batch.AddMessage(msg, nil); err != nil {
			return err
		}
	}
	if d.options.SendTimeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, d.options.SendTimeout)
		defer cancel()
	}
	if err := d.sbSender.SendMessageBatch(ctx, batch, nil); err != nil {
		return fmt.Errorf("failed to send message batch: %w", err)
	}

	return nil
}

// AzSender returns the underlying azservicebus.Sender instance.
func (d *Sender) AzSender() AzServiceBusSender {
	return d.sbSender
}

// SetMessageId sets the ServiceBus message's ID to a user-specified value
func SetMessageId(messageId *string) func(msg *azservicebus.Message) error {
	return func(msg *azservicebus.Message) error {
		msg.MessageID = messageId
		return nil
	}
}

// SetCorrelationId sets the ServiceBus message's correlation ID to a user-specified value
func SetCorrelationId(correlationId *string) func(msg *azservicebus.Message) error {
	return func(msg *azservicebus.Message) error {
		msg.CorrelationID = correlationId
		return nil
	}
}

// SetScheduleAt schedules a message to be enqueued in the future
func SetScheduleAt(t time.Time) func(msg *azservicebus.Message) error {
	return func(msg *azservicebus.Message) error {
		msg.ScheduledEnqueueTime = &t
		return nil
	}
}

// SetMessageDelay schedules a message in the future
func SetMessageDelay(delay time.Duration) func(msg *azservicebus.Message) error {
	return func(msg *azservicebus.Message) error {
		newTime := time.Now().Add(delay)
		msg.ScheduledEnqueueTime = &newTime
		return nil
	}
}

// SetMessageTTL sets the ServiceBus message's TimeToLive to a user-specified value
func SetMessageTTL(ttl time.Duration) func(msg *azservicebus.Message) error {
	return func(msg *azservicebus.Message) error {
		msg.TimeToLive = &ttl
		return nil
	}
}

func getMessageType(mb MessageBody) string {
	var msgType string
	vo := reflect.ValueOf(mb)
	if vo.Kind() == reflect.Ptr {
		msgType = reflect.Indirect(vo).Type().Name()
	} else {
		msgType = vo.Type().Name()
	}

	return msgType
}
