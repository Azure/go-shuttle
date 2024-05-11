package shuttle

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2/metrics/processor"
	"github.com/Azure/go-shuttle/v2/metrics/sender"
)

const defaultHealthCheckInterval = 1 * time.Minute

// HealthChecker performs periodic health checks on the Service Bus Senders and Receivers.
type HealthChecker struct {
	// clients is a map of namespaceName name to azservicebus.Client.
	clients map[string]*azservicebus.Client
	options *HealthCheckerOptions
}

// HealthCheckerOptions configures the HealthChecker.
// HealthCheckInterval defaults to 1 minute if not set or set to <= 0.
// HealthCheckTimeout defaults to HealthChecker.interval if not set or set to 0 or set to be larger than interval.
type HealthCheckerOptions struct {
	// HealthCheckInterval is the time between health checks.
	HealthCheckInterval time.Duration
	// HealthCheckTimeout is the context timeout for each health check
	HealthCheckTimeout time.Duration
}

// NewHealthChecker creates a new HealthChecker with the provided clients.
// clients is a map of namespaceName name to azservicebus.Client.
func NewHealthChecker(clients map[string]*azservicebus.Client, options *HealthCheckerOptions) *HealthChecker {
	if options == nil {
		options = &HealthCheckerOptions{}
	}
	if options.HealthCheckInterval <= 0 {
		options.HealthCheckInterval = defaultHealthCheckInterval
	}
	if options.HealthCheckTimeout <= 0 || options.HealthCheckTimeout > options.HealthCheckInterval {
		options.HealthCheckTimeout = options.HealthCheckInterval
	}

	return &HealthChecker{
		clients: clients,
		options: options,
	}
}

// StartPeriodicHealthCheck starts the periodic health check for the provided HealthCheckable.
// The health check will run on each client in the HealthChecker.
// Stops when the context is canceled.
func (h *HealthChecker) StartPeriodicHealthCheck(ctx context.Context, hc HealthCheckable) {
	for namespaceName, client := range h.clients {
		go func(namespaceName string, client *azservicebus.Client) {
			h.periodicHealthCheck(ctx, hc, namespaceName, client)
		}(namespaceName, client)
	}
}

func (h *HealthChecker) periodicHealthCheck(ctx context.Context, hc HealthCheckable, namespaceName string, client *azservicebus.Client) {
	nextCheck := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(nextCheck)):
			func() {
				sbCtx, cancel := context.WithTimeout(ctx, h.options.HealthCheckTimeout)
				defer cancel()
				err := hc.HealthCheck(sbCtx, namespaceName, client)
				if err != nil {
					getLogger(ctx).Error(fmt.Sprintf("Health check failed for namespace %s: %s", namespaceName, err.Error()))
				}
			}()
			nextCheck = nextCheck.Add(h.options.HealthCheckInterval)
		}
	}
}

// HealthCheckable is an interface for performing health checks on azservicebus.Sender and azservicebus.Receiver.
type HealthCheckable interface {
	HealthCheck(ctx context.Context, namespaceName string, client *azservicebus.Client) error
}

// SenderHealthChecker performs health checks on azservicebus.Sender.
type SenderHealthChecker struct {
	EntityName string
}

// HealthCheck performs a health check on azservicebus.Sender by creating a new message batch.
func (s *SenderHealthChecker) HealthCheck(ctx context.Context, namespaceName string, client *azservicebus.Client) error {
	sbSender, err := client.NewSender(s.EntityName, nil)
	defer func() {
		if sbSender != nil {
			if closeErr := sbSender.Close(ctx); closeErr != nil {
				getLogger(ctx).Error(closeErr.Error())
				err = closeErr
			}
		}
		s.incHealthCheckMetric(namespaceName, err)
	}()
	if err != nil {
		return err
	}
	_, err = sbSender.NewMessageBatch(ctx, nil)
	return err
}

// IncHealthCheckMetric increments the sender health check metric based on the health check result.
func (s *SenderHealthChecker) incHealthCheckMetric(namespaceName string, healthCheckErr error) {
	if healthCheckErr != nil {
		sender.Metric.IncHealthCheckFailureCount(namespaceName, s.EntityName)
	} else {
		sender.Metric.IncHealthCheckSuccessCount(namespaceName, s.EntityName)
	}
}

// ReceiverHealthChecker performs health checks on azservicebus.Receiver.
type ReceiverHealthChecker struct {
	EntityName       string
	SubscriptionName string
}

// HealthCheck performs a health check on the azservicebus.Receiver by peeking a message.
func (r *ReceiverHealthChecker) HealthCheck(ctx context.Context, namespaceName string, client *azservicebus.Client) error {
	sbReceiver, err := r.createReceiver(client)
	defer func() {
		if sbReceiver != nil {
			if closeErr := sbReceiver.Close(ctx); closeErr != nil {
				getLogger(ctx).Error(closeErr.Error())
				err = closeErr
			}
		}
		r.incHealthCheckMetric(namespaceName, err)
	}()
	if err != nil {
		return err
	}
	// note: PeekMessages() does not return an error when the entity is empty
	_, err = sbReceiver.PeekMessages(ctx, 1, nil)
	return err
}

// IncHealthCheckMetric increments the receiver health check metric based on the health check result.
func (r *ReceiverHealthChecker) incHealthCheckMetric(namespaceName string, healthCheckErr error) {
	if healthCheckErr != nil {
		processor.Metric.IncHealthCheckFailureCount(namespaceName, r.EntityName, r.SubscriptionName)
	} else {
		processor.Metric.IncHealthCheckSuccessCount(namespaceName, r.EntityName, r.SubscriptionName)
	}
}

func (r *ReceiverHealthChecker) createReceiver(client *azservicebus.Client) (*azservicebus.Receiver, error) {
	if r.SubscriptionName == "" {
		return client.NewReceiverForQueue(r.EntityName, nil)
	}
	return client.NewReceiverForSubscription(r.EntityName, r.SubscriptionName, nil)
}
