package shuttle

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2/metrics"
	"github.com/Azure/go-shuttle/v2/metrics/processor"
	"github.com/Azure/go-shuttle/v2/metrics/sender"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// NewHealthChecker should use defaultHealthCheckInterval as HealthCheckInterval and HealthCheckTimeout if not set
func TestFunc_NewHealthCheckerNilOptions(t *testing.T) {
	a := require.New(t)
	clients := map[string]*azservicebus.Client{
		"namespace1": {},
		"namespace2": {},
	}

	hc := NewHealthChecker(clients, nil)
	a.Equal(defaultHealthCheckInterval, hc.options.HealthCheckInterval,
		"failed to set HealthCheckInterval, expected: %s, actual: %s", defaultHealthCheckInterval, hc.options.HealthCheckInterval)
	a.Equal(defaultHealthCheckInterval, hc.options.HealthCheckTimeout,
		"failed to set HealthCheckTimeout, expected: %s, actual: %s", defaultHealthCheckInterval, hc.options.HealthCheckTimeout)
}

// NewHealthChecker should use defaultHealthCheckInterval as HealthCheckInterval if options.HealthCheckInterval <= 0
func TestFunc_NewHealthCheckerWithZeroIntervalOptions(t *testing.T) {
	a := require.New(t)
	clients := map[string]*azservicebus.Client{
		"namespace1": {},
		"namespace2": {},
	}

	hc := NewHealthChecker(clients, &HealthCheckerOptions{
		HealthCheckInterval: 0,
	})
	a.Equal(defaultHealthCheckInterval, hc.options.HealthCheckInterval,
		"failed to set HealthCheckInterval, expected: %s, actual: %s", defaultHealthCheckInterval, hc.options.HealthCheckInterval)
	a.Equal(defaultHealthCheckInterval, hc.options.HealthCheckTimeout,
		"failed to set HealthCheckTimeout, expected: %s, actual: %s", defaultHealthCheckInterval, hc.options.HealthCheckTimeout)

	hc = NewHealthChecker(clients, &HealthCheckerOptions{
		HealthCheckInterval: -1,
	})
	a.Equal(defaultHealthCheckInterval, hc.options.HealthCheckInterval,
		"failed to set HealthCheckInterval, expected: %s, actual: %s", defaultHealthCheckInterval, hc.options.HealthCheckInterval)
	a.Equal(defaultHealthCheckInterval, hc.options.HealthCheckTimeout,
		"failed to set HealthCheckTimeout, expected: %s, actual: %s", defaultHealthCheckInterval, hc.options.HealthCheckTimeout)
}

// NewHealthChecker should use interval as HealthCheckTimeout if options.HealthCheckTimeout <= 0
func TestFunc_NewHealthCheckerWithZeroTimeoutOptions(t *testing.T) {
	a := require.New(t)
	clients := map[string]*azservicebus.Client{
		"namespace1": {},
		"namespace2": {},
	}
	interval := 5 * time.Second

	hc := NewHealthChecker(clients, &HealthCheckerOptions{
		HealthCheckInterval: interval,
	})
	a.Equal(interval, hc.options.HealthCheckInterval,
		"failed to set HealthCheckInterval, expected: %s, actual: %s", interval, hc.options.HealthCheckInterval)
	a.Equal(interval, hc.options.HealthCheckTimeout,
		"failed to set HealthCheckTimeout, expected: %s, actual: %s", interval, hc.options.HealthCheckTimeout)

	hc = NewHealthChecker(clients, &HealthCheckerOptions{
		HealthCheckInterval: interval,
		HealthCheckTimeout:  -1,
	})
	a.Equal(interval, hc.options.HealthCheckInterval,
		"failed to set HealthCheckInterval, expected: %s, actual: %s", interval, hc.options.HealthCheckInterval)
	a.Equal(interval, hc.options.HealthCheckTimeout,
		"failed to set HealthCheckTimeout, expected: %s, actual: %s", interval, hc.options.HealthCheckTimeout)
}

// NewHealthChecker should use interval as HealthCheckTimeout if options.HealthCheckTimeout > interval
func TestFunc_NewHealthCheckerWithOptions(t *testing.T) {
	a := require.New(t)
	clients := map[string]*azservicebus.Client{
		"namespace1": {},
		"namespace2": {},
	}
	interval := 5 * time.Second
	timeout := 10 * time.Second

	hc := NewHealthChecker(clients, &HealthCheckerOptions{
		HealthCheckInterval: interval,
		HealthCheckTimeout:  timeout,
	})
	a.Equal(interval, hc.options.HealthCheckInterval,
		"failed to set HealthCheckInterval, expected: %s, actual: %s", interval, hc.options.HealthCheckInterval)
	a.Equal(interval, hc.options.HealthCheckTimeout,
		"failed to set HealthCheckTimeout, expected: %s, actual: %s", interval, hc.options.HealthCheckTimeout)

	timeout = 1 * time.Second
	hc = NewHealthChecker(clients, &HealthCheckerOptions{
		HealthCheckTimeout: timeout,
	})
	a.Equal(defaultHealthCheckInterval, hc.options.HealthCheckInterval,
		"failed to set HealthCheckInterval, expected: %s, actual: %s", defaultHealthCheckInterval, hc.options.HealthCheckInterval)
	a.Equal(timeout, hc.options.HealthCheckTimeout,
		"failed to set HealthCheckTimeout, expected: %s, actual: %s", interval, hc.options.HealthCheckTimeout)

	timeout = 10 * time.Minute
	hc = NewHealthChecker(clients, &HealthCheckerOptions{
		HealthCheckTimeout: timeout,
	})
	a.Equal(defaultHealthCheckInterval, hc.options.HealthCheckInterval,
		"failed to set HealthCheckInterval, expected: %s, actual: %s", defaultHealthCheckInterval, hc.options.HealthCheckInterval)
	a.Equal(defaultHealthCheckInterval, hc.options.HealthCheckTimeout,
		"failed to set HealthCheckTimeout, expected: %s, actual: %s", defaultHealthCheckInterval, hc.options.HealthCheckTimeout)
}

// With a failing connection and a check interval of 20ms,
// the health checker should have 5 failed checks after 90ms
func TestHealthChecker_FailConnectionCheck(t *testing.T) {
	type testCase struct {
		entity       string
		subscription string
	}
	a := require.New(t)
	metrics.Register(prometheus.NewRegistry())
	numClients := 3
	clients, err := getTestClients(numClients)
	a.NoError(err)
	hc := NewHealthChecker(clients, &HealthCheckerOptions{
		HealthCheckInterval: 20 * time.Millisecond,
		HealthCheckTimeout:  5 * time.Millisecond,
	})
	a.Equal(20*time.Millisecond, hc.options.HealthCheckInterval)
	a.Equal(5*time.Millisecond, hc.options.HealthCheckTimeout)

	for _, tc := range []testCase{
		{entity: "testQueue", subscription: ""},
		{entity: "testTopic", subscription: "testSubscription"},
	} {
		shc := &SenderHealthChecker{EntityName: tc.entity}
		rhc := &ReceiverHealthChecker{EntityName: tc.entity}
		if tc.subscription != "" {
			rhc.SubscriptionName = tc.subscription
		}

		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Millisecond)
		defer cancel()
		hc.StartPeriodicHealthCheck(ctx, shc)
		hc.StartPeriodicHealthCheck(ctx, rhc)

		time.Sleep(100 * time.Millisecond)

		for i := 0; i < numClients; i++ {
			namespaceName := fmt.Sprintf("myservicebus-%d.servicebus.windows.net", i)
			informer := sender.NewInformer()
			senderSuccessCount, err := informer.GetHealthCheckSuccessCount(namespaceName, tc.entity)
			a.NoError(err)
			a.Equal(0, int(senderSuccessCount), "sender success count mismatch, namespace: %s, entity: %s", namespaceName, tc.entity)
			senderFailureCount, err := informer.GetHealthCheckFailureCount(namespaceName, tc.entity)
			a.NoError(err)
			a.Equal(5, int(senderFailureCount), "sender failure count mismatch, namespace: %s, entity: %s", namespaceName, tc.entity)

			processorInformer := processor.NewInformer()
			receiverSuccessCount, err := processorInformer.GetHealthCheckSuccessCount(namespaceName, tc.entity, tc.subscription)
			a.NoError(err)
			a.Equal(0, int(receiverSuccessCount), "receiver success count mismatch, namespace: %s, entity: %s, subscription %s", namespaceName, tc.entity, tc.subscription)
			receiverFailureCount, err := processorInformer.GetHealthCheckFailureCount(namespaceName, tc.entity, tc.subscription)
			a.NoError(err)
			a.Equal(5, int(receiverFailureCount), "receiver failure count mismatch, namespace: %s, entity: %s", namespaceName, tc.entity, tc.subscription)
		}
	}
}

// With a successful connection and a check interval of 20ms,
// the health checker should have 5 successful checks after 90ms
func TestHealthChecker_SuccessConnectionCheck(t *testing.T) {
	a := require.New(t)
	metrics.Register(prometheus.NewRegistry())
	numClients := 3
	clients, err := getTestClients(numClients)
	a.NoError(err)
	entity := "testEntity"
	interval := 20 * time.Millisecond

	hc := NewHealthChecker(clients, &HealthCheckerOptions{
		HealthCheckInterval: interval,
	})
	a.Equal(interval, hc.options.HealthCheckInterval)
	a.Equal(interval, hc.options.HealthCheckTimeout)

	fhc := &fakeHealthChecker{entityName: entity}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Millisecond)
	defer cancel()
	hc.StartPeriodicHealthCheck(ctx, fhc)

	time.Sleep(100 * time.Millisecond)
	for i := 0; i < numClients; i++ {
		namespaceName := fmt.Sprintf("myservicebus-%d.servicebus.windows.net", i)
		informer := sender.NewInformer()
		senderSuccessCount, err := informer.GetHealthCheckSuccessCount(namespaceName, entity)
		a.NoError(err)
		a.Equal(5, int(senderSuccessCount), "sender success count mismatch, namespace: %s, entity: %s", namespaceName, entity)
		senderFailureCount, err := informer.GetHealthCheckFailureCount(namespaceName, entity)
		a.NoError(err)
		a.Equal(0, int(senderFailureCount), "sender failure count mismatch, namespace: %s, entity: %s", namespaceName, entity)

		processorInformer := processor.NewInformer()
		receiverSuccessCount, err := processorInformer.GetHealthCheckSuccessCount(namespaceName, entity, "")
		a.NoError(err)
		a.Equal(5, int(receiverSuccessCount), "receiver success count mismatch, namespace: %s, entity: %s", namespaceName, entity)
		receiverFailureCount, err := processorInformer.GetHealthCheckFailureCount(namespaceName, entity, "")
		a.NoError(err)
		a.Equal(0, int(receiverFailureCount), "receiver failure count mismatch, namespace: %s, entity: %s", namespaceName, entity)
	}
}

func getTestClients(n int) (map[string]*azservicebus.Client, error) {
	clients := make(map[string]*azservicebus.Client)
	tokenCredential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	for i := 0; i < n; i++ {
		namespaceName := fmt.Sprintf("myservicebus-%d.servicebus.windows.net", i)
		clientOptions := &azservicebus.ClientOptions{
			RetryOptions: azservicebus.RetryOptions{
				MaxRetries: -1,
			},
		}
		client, err := azservicebus.NewClient(namespaceName, tokenCredential, clientOptions)
		if err != nil {
			return nil, err
		}
		clients[namespaceName] = client
	}
	return clients, nil
}

type fakeHealthChecker struct {
	entityName       string
	subscriptionName string
}

func (f *fakeHealthChecker) HealthCheck(_ context.Context, _ *azservicebus.Client, _ context.CancelFunc) error {
	return nil
}

func (f *fakeHealthChecker) IncHealthCheckMetric(namespaceName string, healthCheckErr error) {
	shc := &SenderHealthChecker{EntityName: f.entityName}
	shc.IncHealthCheckMetric(namespaceName, healthCheckErr)
	rhc := &ReceiverHealthChecker{EntityName: f.entityName, SubscriptionName: f.subscriptionName}
	rhc.IncHealthCheckMetric(namespaceName, healthCheckErr)
}
