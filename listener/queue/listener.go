package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/handlers"
	"github.com/Azure/go-shuttle/message"
	"github.com/devigned/tab"
)

const (
	defaultQueueName = "default"
)

// Listener is a struct to contain service bus entities relevant to subscribing to a publisher queue
type Listener struct {
	namespace           *servicebus.Namespace
	queueEntity         *servicebus.QueueEntity
	queueListener  		*servicebus.Queue
	listenerHandle      *servicebus.ListenerHandle
	topicName           string
	queueName    		string
	maxDeliveryCount    int32
	lockRenewalInterval *time.Duration
	lockDuration        time.Duration
	prefetchCount       *uint32
	maxConcurrency      *int
}

// QueueListener returns the servicebus.Queue that the listener is setup with
func (l *Listener) QueueListener() *servicebus.Queue {
	return l.queueListener
}

// Queue returns servicebus.QueueEntity that the listener is setup with
func (l *Listener) Queue() *servicebus.QueueEntity {
	return l.queueEntity
}

// Namespace returns the servicebus.Namespace that the listener is setup with
func (l *Listener) Namespace() *servicebus.Namespace {
	return l.namespace
}

// New creates a new service bus listener
func New(opts ...ManagementOption) (*Listener, error) {
	ns, err := servicebus.NewNamespace()
	if err != nil {
		return nil, err
	}
	listener := &Listener{namespace: ns}
	for _, opt := range opts {
		err := opt(listener)
		if err != nil {
			return nil, err
		}
	}
	if listener.queueName == "" {
		listener.queueName = defaultQueueName
	}

	return listener, nil
}

func setQueueEntity(ctx context.Context, l *Listener) error {
	if l.queueEntity != nil {
		return nil
	}
	queueEntity, err := getQueueEntity(ctx, l.queueName, l.namespace)
	if err != nil {
		return fmt.Errorf("failed to get queue: %w", err)
	}
	l.queueEntity = queueEntity
	return nil
}

// Listen waits for a message from the Service Bus Topic subscription
func (l *Listener) Listen(ctx context.Context, handler message.Handler, queueName string, opts ...Option) error {
	ctx, span := tab.StartSpan(ctx, "go-shuttle.queue.listener.Listen")
	defer span.End()
	l.queueName = queueName
	// apply listener options
	for _, opt := range opts {
		err := opt(l)
		if err != nil {
			return err
		}
	}
	if err := setQueueEntity(ctx, l); err != nil {
		return err
	}
	if err := ensureQueueListener(l); err != nil {
		return err
	}
	// Generate new topic client
	queue, err := l.namespace.NewQueue(l.queueEntity.Name)
	if err != nil {
		return fmt.Errorf("failed to create new queue %s: %w", l.queueEntity.Name, err)
	}
	defer func() {
		_ = queue.Close(ctx)
	}()

	var receiverOpts []servicebus.ReceiverOption
	if l.prefetchCount != nil {
		receiverOpts = append(receiverOpts, servicebus.ReceiverWithPrefetchCount(*l.prefetchCount))
	}
	handlerConcurrency := 1
	if l.maxConcurrency != nil {
		handlerConcurrency = *l.maxConcurrency
	}
	queueReceiver, err := queue.NewReceiver(ctx, receiverOpts...)
	if err != nil {
		return fmt.Errorf("failed to create new subscription receiver %s: %w", l.queueEntity.Name, err)
	}
	listenerHandle := queueReceiver.Listen(ctx,
		handlers.NewConcurrent(handlerConcurrency,
			handlers.NewDeadlineContext(
				handlers.NewPeekLockRenewer(l.lockRenewalInterval, queue,
					handlers.NewShuttleAdapter(handler)),
			)))
	l.listenerHandle = listenerHandle
	<-listenerHandle.Done()

	if err := queueReceiver.Close(ctx); err != nil {
		return fmt.Errorf("error shutting down service bus queue listener. %w", err)
	}
	return listenerHandle.Err()
}

// Close closes the listener if an active listener exists
func (l *Listener) Close(ctx context.Context) error {
	if l.listenerHandle == nil {
		return errors.New("no active listener. cannot close")
	}
	if err := l.listenerHandle.Close(ctx); err != nil {
		l.listenerHandle = nil
		return fmt.Errorf("error shutting down service bus subscription. %w", err)
	}
	return nil
}

// GetActiveMessageCount gets the active message count of a topic subscription
// WARNING: GetActiveMessageCount is 10 times expensive than a call to receive a message
func (l *Listener) GetActiveMessageCount(ctx context.Context, queueName string) (int32, error) {
	if l.queueEntity == nil {
		return 0, fmt.Errorf("entity of queue is nil")
	}

	if l.queueEntity.CountDetails == nil || l.queueEntity.CountDetails.ActiveMessageCount == nil {
		return 0, fmt.Errorf("active message count is not available in the entity of queue %q", queueName)
	}

	return *l.queueEntity.CountDetails.ActiveMessageCount, nil
}

func getQueueEntity(ctx context.Context, queueName string, namespace *servicebus.Namespace) (*servicebus.QueueEntity, error) {
	qm := namespace.NewQueueManager()
	return qm.Get(ctx, queueName)
}

func ensureQueueListener(l *Listener) error {
	if l.queueListener != nil {
		return nil
	}
	if l.queueName == "" {
		l.queueName = defaultQueueName
	}
	queueListener, err := l.getQueueListener()
	if err != nil {
		return fmt.Errorf("failed to get queueListener: %w", err)
	}
	l.queueListener = queueListener
	return nil
}

func (l *Listener) getQueueListener() (*servicebus.Queue, error) {

	queue, err := l.namespace.NewQueue(l.queueEntity.Name)
	if err != nil {
		return nil, fmt.Errorf("creating queue manager failed: %w", err)
	}

	return queue, nil
}
