package shuttle

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"

	"github.com/Azure/go-shuttle/v2/metrics/sender"
)

const (
	msgTypeField       = "type"
	defaultSendTimeout = 30 * time.Second
)

// MessageBody is a type to represent that an input message body can be of any type
type MessageBody any

// SendAsBatchOptions contains options for the SendAsBatch method
type SendAsBatchOptions struct {
	// AllowMultipleBatch when true, allows splitting large message arrays into multiple batches.
	// When false, behaves like the original SendMessageBatch method.
	// Default: false
	AllowMultipleBatch bool
}

// AzServiceBusSender is satisfied by *azservicebus.Sender
type AzServiceBusSender interface {
	SendMessage(ctx context.Context, message *azservicebus.Message, options *azservicebus.SendMessageOptions) error
	SendMessageBatch(ctx context.Context, batch *azservicebus.MessageBatch, options *azservicebus.SendMessageBatchOptions) error
	NewMessageBatch(ctx context.Context, options *azservicebus.MessageBatchOptions) (*azservicebus.MessageBatch, error)
	Close(ctx context.Context) error
}

// Sender contains an SBSender used to send the message to the ServiceBus queue and a Marshaller used to marshal any struct into a ServiceBus message
type Sender struct {
	// sbSender is responsible for message sending. It is protected by a mutex to prevent race conditions.
	// Any usage of sbSender must consider the mutex and take the appropriate lock.
	sbSender AzServiceBusSender
	// mu is used to prevent race conditions when changing the sbSender instance.
	// Use mu.Lock() only during FailOver and mu.RLock() for all other operations
	mu      sync.RWMutex
	options *SenderOptions
}

type SenderOptions struct {
	// Marshaller will be used to marshall the messageBody to the azservicebus.Message Body property
	// defaults to DefaultJSONMarshaller
	Marshaller Marshaller
	// EnableTracingPropagation automatically applies WithTracePropagation option on all message sent through this sender
	EnableTracingPropagation bool
	// SendTimeout is the timeout value used on the context that sends messages
	// Defaults to 30 seconds if not set or 0
	// Disabled when set to a negative value
	SendTimeout time.Duration
}

// NewSender takes in a Sender and a Marshaller to create a new object that can send messages to the ServiceBus queue
func NewSender(sender AzServiceBusSender, options *SenderOptions) *Sender {
	if options == nil {
		options = &SenderOptions{}
	}
	if options.Marshaller == nil {
		options.Marshaller = &DefaultJSONMarshaller{}
	}
	if options.SendTimeout == 0 {
		options.SendTimeout = defaultSendTimeout
	}
	return &Sender{sbSender: sender, options: options}
}

// SendMessage sends a payload on the bus.
// the MessageBody is marshalled and set as the message body.
func (d *Sender) SendMessage(ctx context.Context, mb MessageBody, options ...func(msg *azservicebus.Message) error) error {
	// Check if there is a context error before doing anything since
	// we rely on context failures to detect if the sender is dead.
	if ctx.Err() != nil {
		return fmt.Errorf("failed to send message: %w", ctx.Err())
	}

	msg, err := d.ToServiceBusMessage(ctx, mb, options...)
	if err != nil {
		return err
	}
	if d.options.SendTimeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, d.options.SendTimeout)
		defer cancel()
	}

	errChan := make(chan error)
	go func() {
		err := d.sendMessage(ctx, msg, nil) // sendMessageOptions currently does nothing
		if err != nil {
			errChan <- fmt.Errorf("failed to send message: %w", err)
		} else {
			errChan <- nil
		}
	}()

	select {
	case <-ctx.Done():
		sender.Metric.IncSendMessageFailureCount()
		return fmt.Errorf("failed to send message: %w", ctx.Err())
	case err := <-errChan:
		if err == nil {
			sender.Metric.IncSendMessageSuccessCount()
		} else {
			sender.Metric.IncSendMessageFailureCount()
		}
		return err
	}

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

// SendAsBatch sends the array of azservicebus messages as batches.
// When options.AllowMultipleBatch is true, large message arrays are split into multiple batches.
// When options.AllowMultipleBatch is false, behaves like SendMessageBatch (fails if messages don't fit in single batch).
//
// Example usage:
//   // For backward compatibility (single batch only)
//   err := sender.SendAsBatch(ctx, messages, &SendAsBatchOptions{AllowMultipleBatch: false})
//
//   // For large message arrays (multiple batches allowed)
//   err := sender.SendAsBatch(ctx, messages, &SendAsBatchOptions{AllowMultipleBatch: true})
//
//   // With nil options (defaults to AllowMultipleBatch: false)
//   err := sender.SendAsBatch(ctx, messages, nil)
func (d *Sender) SendAsBatch(ctx context.Context, messages []*azservicebus.Message, options *SendAsBatchOptions) error {
	// Check if there is a context error before doing anything since
	// we rely on context failures to detect if the sender is dead.
	if ctx.Err() != nil {
		return fmt.Errorf("failed to send message batch: %w", ctx.Err())
	}

	if options == nil {
		options = &SendAsBatchOptions{AllowMultipleBatch: false}
	}

	if len(messages) == 0 {
		// For backward compatibility, still create and send an empty batch
		// when AllowMultipleBatch is false (original SendMessageBatch behavior)
		if !options.AllowMultipleBatch {
			return d.sendSingleBatch(ctx, messages)
		}
		return nil // Nothing to send for multiple batch mode
	}

	if !options.AllowMultipleBatch {
		// Original behavior: try to fit all messages in a single batch
		return d.sendSingleBatch(ctx, messages)
	}

	// Multiple batch behavior: split messages across batches as needed
	return d.sendMultipleBatches(ctx, messages)
}

// sendSingleBatch implements the original SendMessageBatch behavior
func (d *Sender) sendSingleBatch(ctx context.Context, messages []*azservicebus.Message) error {
	batch, err := d.newMessageBatch(ctx, &azservicebus.MessageBatchOptions{})
	if err != nil {
		return err
	}

	for _, msg := range messages {
		if err := batch.AddMessage(msg, nil); err != nil {
			return err
		}
	}

	// Apply timeout just before sending (matching original behavior)
	if d.options.SendTimeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, d.options.SendTimeout)
		defer cancel()
	}

	errChan := make(chan error)
	go func() {
		if err := d.sendMessageBatch(ctx, batch, nil); err != nil {
			errChan <- fmt.Errorf("failed to send message batch: %w", err)
		} else {
			errChan <- nil
		}
	}()

	select {
	case <-ctx.Done():
		sender.Metric.IncSendMessageFailureCount()
		return fmt.Errorf("failed to send message batch: %w", ctx.Err())
	case err := <-errChan:
		if err == nil {
			sender.Metric.IncSendMessageSuccessCount()
		} else {
			sender.Metric.IncSendMessageFailureCount()
		}
		return err
	}
}

// sendMultipleBatches splits messages across multiple batches as needed
func (d *Sender) sendMultipleBatches(ctx context.Context, messages []*azservicebus.Message) error {
	var batches []*azservicebus.MessageBatch
	currentBatch, err := d.newMessageBatch(ctx, &azservicebus.MessageBatchOptions{})
	if err != nil {
		return err
	}

	// Split messages into batches
	for _, msg := range messages {
		err := currentBatch.AddMessage(msg, nil)
		if err != nil {
			// Current batch is full, start a new one
			batches = append(batches, currentBatch)
			currentBatch, err = d.newMessageBatch(ctx, &azservicebus.MessageBatchOptions{})
			if err != nil {
				return err
			}
			// Try adding the message to the new batch
			if err := currentBatch.AddMessage(msg, nil); err != nil {
				return fmt.Errorf("message too large for batch: %w", err)
			}
		}
	}

	// Add the final batch if it has messages
	if currentBatch != nil {
		batches = append(batches, currentBatch)
	}

	// Apply timeout for sending all batches
	if d.options.SendTimeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, d.options.SendTimeout)
		defer cancel()
	}

	// Send all batches
	for i, batch := range batches {
		errChan := make(chan error)
		go func(b *azservicebus.MessageBatch) {
			if err := d.sendMessageBatch(ctx, b, nil); err != nil {
				errChan <- fmt.Errorf("failed to send message batch %d: %w", i+1, err)
			} else {
				errChan <- nil
			}
		}(batch)

		select {
		case <-ctx.Done():
			sender.Metric.IncSendMessageFailureCount()
			return fmt.Errorf("failed to send message batch %d: %w", i+1, ctx.Err())
		case err := <-errChan:
			if err != nil {
				sender.Metric.IncSendMessageFailureCount()
				return err
			}
			sender.Metric.IncSendMessageSuccessCount()
		}
	}

	return nil
}

// SendMessageBatch sends the array of azservicebus messages as a batch.
// Deprecated: Use SendAsBatch instead. This method will be removed in a future version.
func (d *Sender) SendMessageBatch(ctx context.Context, messages []*azservicebus.Message) error {
	return d.SendAsBatch(ctx, messages, &SendAsBatchOptions{AllowMultipleBatch: false})
}

func (d *Sender) sendMessage(ctx context.Context, msg *azservicebus.Message, options *azservicebus.SendMessageOptions) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.sbSender.SendMessage(ctx, msg, options)
}

func (d *Sender) sendMessageBatch(ctx context.Context, batch *azservicebus.MessageBatch, options *azservicebus.SendMessageBatchOptions) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.sbSender.SendMessageBatch(ctx, batch, options)
}

func (d *Sender) newMessageBatch(ctx context.Context, options *azservicebus.MessageBatchOptions) (*azservicebus.MessageBatch, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.sbSender.NewMessageBatch(ctx, options)
}

// AzSender returns the underlying azservicebus.Sender instance.
func (d *Sender) AzSender() AzServiceBusSender {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.sbSender
}

// SetAzSender sets the underlying azservicebus.Sender instance to the provided one.
// All ongoing send operations will continue to use the old sender instance,
// while new send operations will use the new sender instance.
func (d *Sender) SetAzSender(sender AzServiceBusSender) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sbSender = sender
}

// Deprecated: use SetAzSender.
// FailOver sets the underlying azservicebus.Sender instance to the provided one.
func (d *Sender) FailOver(sender AzServiceBusSender) {
	d.SetAzSender(sender)
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

// SetSubject sets the ServiceBus message's Subject property to a user-specified value
func SetSubject(subject string) func(msg *azservicebus.Message) error {
	return func(msg *azservicebus.Message) error {
		msg.Subject = &subject
		return nil
	}
}

// SetTo sets the ServiceBus message's To property to a user-specified value
func SetTo(to string) func(msg *azservicebus.Message) error {
	return func(msg *azservicebus.Message) error {
		msg.To = &to
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
