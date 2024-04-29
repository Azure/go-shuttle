package shuttle

import (
	"context"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2/metrics/processor"
	"github.com/Azure/go-shuttle/v2/metrics/sender"
)

type HealthCheckFunc func(ctx context.Context, namespace string, client *azservicebus.Client) error

type HealthChecker struct {
	clients      map[string]*azservicebus.Client
	entity       string
	subscription string
	interval     time.Duration
}

// NewHealthChecker creates a new HealthChecker
func NewHealthChecker(clients map[string]*azservicebus.Client, entity, subscription string, interval time.Duration) *HealthChecker {
	return &HealthChecker{
		clients:      clients,
		entity:       entity,
		subscription: subscription,
		interval:     interval,
	}
}

// StartSenderPeriodicHealthCheck starts a periodic health check for the sender
// stops when the context is cancelled
func (h *HealthChecker) StartSenderPeriodicHealthCheck(ctx context.Context) {
	for namespace, client := range h.clients {
		go h.periodicHealthCheck(ctx, h.senderHealthCheck, namespace, client)
	}
}

// StartReceiverPeriodicHealthCheck starts a periodic health check for the receiver
// stops when the context is cancelled
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
		sender.Metric.IncConsecutiveConnectionFailureCount(namespace, h.entity)
		return err
	}
	_, err = s.NewMessageBatch(ctx, nil)
	if err != nil {
		sender.Metric.IncConsecutiveConnectionFailureCount(namespace, h.entity)
		return err
	}
	sender.Metric.IncConsecutiveConnectionSuccessCount(namespace, h.entity)
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
		processor.Metric.IncConsecutiveConnectionFailureCount(namespace, h.entity, h.subscription)
		return err
	}
	_, err = r.PeekMessages(ctx, 1, nil)
	if err != nil {
		processor.Metric.IncConsecutiveConnectionFailureCount(namespace, h.entity, h.subscription)
		return err
	}
	processor.Metric.IncConsecutiveConnectionSuccessCount(namespace, h.entity, h.subscription)
	return r.Close(ctx)

}
