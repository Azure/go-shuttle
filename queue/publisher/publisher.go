package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Azure/go-shuttle/common/baseinterfaces"
	"time"

	common "github.com/Azure/azure-amqp-common-go/v3"
	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/common/errorhandling"
	"github.com/Azure/go-shuttle/internal/reflection"
	"github.com/Azure/go-shuttle/prometheus/publisher"
	"github.com/devigned/tab"
)

type QueuePublisher interface {
	baseinterfaces.BasePublisher
	AppendQueueManagementOption(option servicebus.QueueManagementOption)
}

// Publisher is a struct to contain service bus entities relevant to publishing to a queue
type Publisher struct {
	baseinterfaces.PublisherSettings
	queue                  *servicebus.Queue
	queueManagementOptions []servicebus.QueueManagementOption
}

func (p *Publisher) AppendQueueManagementOption(option servicebus.QueueManagementOption) {
	p.queueManagementOptions = append(p.queueManagementOptions, option)
}

// New creates a new service bus publisher
func New(ctx context.Context, queueName string, opts ...ManagementOption) (*Publisher, error) {
	ctx, s := tab.StartSpan(ctx, "go-shuttle.publisher.New")
	defer s.End()
	ns, err := servicebus.NewNamespace()
	if err != nil {
		return nil, err
	}
	publisher := &Publisher{PublisherSettings: baseinterfaces.PublisherSettings{}}
	publisher.SetNamespace(ns)
	for _, opt := range opts {
		err := opt(publisher)
		if err != nil {
			return nil, err
		}
	}

	queueEntity, err := ensureQueue(ctx, queueName, publisher.Namespace(), publisher.queueManagementOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue: %w", err)
	}
	if err = publisher.initQueue(queueEntity.Name); err != nil {
		return nil, fmt.Errorf("failed to create new queue %s: %w", queueEntity.Name, err)
	}
	return publisher, nil
}

// Publish publishes to the pre-configured Service Bus queue
func (p *Publisher) Publish(ctx context.Context, msg interface{}, opts ...Option) error {
	ctx, s := tab.StartSpan(ctx, "go-shuttle.publisher.Publish")
	defer s.End()
	msgJSON, err := json.Marshal(msg)

	// adding in user properties to enable filtering on listener side
	sbMsg := servicebus.NewMessageFromString(string(msgJSON))
	sbMsg.UserProperties = make(map[string]interface{})
	sbMsg.UserProperties["type"] = reflection.GetType(msg)

	// add in custom headers setup at initialization time
	for headerName, headerKey := range p.Headers() {
		val := reflection.GetReflectionValue(msg, headerKey)
		if val != nil {
			sbMsg.UserProperties[headerName] = val
		}
	}

	for _, opt := range opts {
		err := opt(sbMsg)
		if err != nil {
			return err
		}
	}

	err = p.queue.Send(ctx, sbMsg)
	if err == nil {
		return nil
	}
	// recover + retry
	if recErr := p.tryRecoverQueue(ctx, err); recErr != nil {
		publisher.Metrics.IncConnectionRecoveryFailure(err)
		return fmt.Errorf("failed to recover queue on send failure %s. recoveryError : %w, sendError: %s", p.queue.Name, recErr, err)
	}
	publisher.Metrics.IncConnectionRecoverySuccess(err)
	if err = p.queue.Send(ctx, sbMsg); err != nil {
		return fmt.Errorf("failed to send message to queue %s after recovery: %w", p.queue.Name, err)
	}
	return nil
}

func (p *Publisher) Close(ctx context.Context) error {
	ctx, s := tab.StartSpan(ctx, "go-shuttle.publisher.Close")
	defer s.End()
	return p.queue.Close(ctx)
}

func (p *Publisher) tryRecoverQueue(ctx context.Context, sendError error) error {
	ctx, s := tab.StartSpan(ctx, "go-shuttle.publisher.tryRecoverQueue", tab.StringAttribute("error", sendError.Error()))
	defer s.End()
	if errorhandling.IsConnectionDead(sendError) {
		if err := p.initQueue(p.queue.Name); err != nil {
			return fmt.Errorf("failed to init queue on recovery: %w", err)
		}

		return nil
	}
	return fmt.Errorf("error is not identified as recoverable: %w", sendError)
}

func ensureQueue(ctx context.Context, name string, namespace *servicebus.Namespace, opts ...servicebus.QueueManagementOption) (*servicebus.QueueEntity, error) {
	attempt := 1
	qm := namespace.NewQueueManager()
	ensure := func() (interface{}, error) {
		ctx, span := tab.StartSpan(ctx, "go-shuttle.publisher.ensureQueue")
		span.AddAttributes(tab.Int64Attribute("retry.attempt", int64(attempt)))
		qe, err := qm.Get(ctx, name)
		if err == nil {
			return qe, nil
		}
		qe, err = qm.Put(ctx, name, opts...)
		if err != nil {
			attempt++
			tab.For(ctx).Error(err)
			// let all errors be retryable for now. application only hit this once on queue creation.
			return nil, common.Retryable(err.Error())
		}
		return qe, nil
	}
	entity, err := common.Retry(5, 1*time.Second, ensure)
	if err != nil {
		tab.For(ctx).Error(err)
		return nil, err
	}
	return entity.(*servicebus.QueueEntity), nil
}

func (p *Publisher) initQueue(name string) error {
	queue, err := p.Namespace().NewQueue(name)
	if err != nil {
		return fmt.Errorf("failed to create new queue %s: %w", name, err)
	}
	p.queue = queue
	return nil
}
