package shuttle

import (
	"context"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2/metrics/processor"
	"github.com/Azure/go-shuttle/v2/metrics/sender"
)

const (
	defaultHealthCheckTimeout = 5 * time.Second
)

type HealthCheckFunc func(ctx context.Context, namespace string, client *azservicebus.Client) error

// HealthChecker performs periodic health checks on the Service Bus Senders and Receivers.
// It uses azservicebus.Sender.NewMessageBatch() and azservicebus.Receiver.PeekMessages() to perform the health checks.
type HealthChecker struct {
	// clients is a map of namespace name to azservicebus.Client.
	clients map[string]*azservicebus.Client
	// entity is the name of the queue or topic.
	entity string
	// subscription is the name of the subscription. Leave empty for queues and senders.
	subscription string
	// interval is the time between health checks.
	interval time.Duration
}

// HealthCheckerOptions configures the HealthChecker.
// HealthCheckTimeout defaults to 5 seconds if not set or set to 0. Disabled when set to a negative value.
type HealthCheckerOptions struct {
	// HealthCheckTimeout is the context timeout for each health check
	HealthCheckTimeout time.Duration
}

// NewHealthChecker creates a new HealthChecker with the provided clients, entity, subscription, and interval.
// clients is a map of namespace name to azservicebus.Client.
func NewHealthChecker(clients map[string]*azservicebus.Client, entity, subscription string, interval time.Duration, options *HealthCheckerOptions) *HealthChecker {
	if options == nil {
		options = &HealthCheckerOptions{}
	}
	if options.HealthCheckTimeout == 0 {
		options.HealthCheckTimeout = defaultHealthCheckTimeout
	}

	return &HealthChecker{
		clients:      clients,
		entity:       entity,
		subscription: subscription,
		interval:     interval,
	}
}

// StartSenderPeriodicHealthCheck starts a periodic health check for the sender.
// It uses azservicebus.Sender.NewMessageBatch() to perform the health check.
// Stops when the context is cancelled.
func (h *HealthChecker) StartSenderPeriodicHealthCheck(ctx context.Context) {
	for namespace, client := range h.clients {
		go h.periodicHealthCheck(ctx, h.senderHealthCheck, namespace, client)
	}
}

// StartReceiverPeriodicHealthCheck starts a periodic health check for the receiver
// It uses azservicebus.Receiver.PeekMessages() to perform the health check.
// Stops when the context is cancelled.
func (h *HealthChecker) StartReceiverPeriodicHealthCheck(ctx context.Context) {
	for namespace, client := range h.clients {
		go func(namespace string, client *azservicebus.Client) {
			go h.periodicHealthCheck(ctx, h.receiverHealthCheck, namespace, client)
		}(namespace, client)
	}
}

func (h *HealthChecker) periodicHealthCheck(ctx context.Context, healthCheckFunc HealthCheckFunc, namespace string, client *azservicebus.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(h.interval):
			if err := healthCheckFunc(ctx, namespace, client); err != nil {
				log(ctx, err)
			}
		}
	}
}

func (h *HealthChecker) senderHealthCheck(ctx context.Context, namespace string, client *azservicebus.Client) error {
	s, err := client.NewSender(h.entity, nil)
	if err != nil {
		sender.Metric.IncHealthCheckFailureCount(namespace, h.entity)
		return err
	}
	_, err = s.NewMessageBatch(ctx, nil)
	if err != nil {
		sender.Metric.IncHealthCheckFailureCount(namespace, h.entity)
		return err
	}
	sender.Metric.IncHealthCheckSuccessCount(namespace, h.entity)
	return s.Close(ctx)
}

func (h *HealthChecker) receiverHealthCheck(ctx context.Context, namespace string, client *azservicebus.Client) error {
	var r *azservicebus.Receiver
	var err error
	if h.subscription != "" {
		r, err = client.NewReceiverForSubscription(h.entity, h.subscription, nil)
	} else {
		r, err = client.NewReceiverForQueue(h.entity, nil)
	}
	if err != nil {
		processor.Metric.IncHealthCheckFailureCount(namespace, h.entity, h.subscription)
		return err
	}
	_, err = r.PeekMessages(ctx, 1, nil)
	if err != nil {
		processor.Metric.IncHealthCheckFailureCount(namespace, h.entity, h.subscription)
		return err
	}
	processor.Metric.IncHealthCheckSuccessCount(namespace, h.entity, h.subscription)
	return r.Close(ctx)
}
