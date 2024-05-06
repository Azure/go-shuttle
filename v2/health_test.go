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

// NewHealthChecker should use interval as HealthCheckTimeout if not set
func TestFunc_NewHealthCheckerNilOptions(t *testing.T) {
	clients := map[string]*azservicebus.Client{
		"namespace1": {},
		"namespace2": {},
	}
	entity := "testEntity"
	subscription := "testSubscription"
	interval := 5 * time.Second

	hc := NewHealthChecker(clients, entity, subscription, interval, nil)
	if hc.options.HealthCheckTimeout != interval {
		t.Errorf("failed to set HealthCheckTimeout, expected: %s, actual: %s", interval, hc.options.HealthCheckTimeout)
	}
}

// NewHealthChecker should use interval as HealthCheckTimeout if options.HealthCheckTimeout > interval
func TestFunc_NewHealthCheckerWithOptions(t *testing.T) {
	clients := map[string]*azservicebus.Client{
		"namespace1": {},
		"namespace2": {},
	}
	entity := "testEntity"
	subscription := "testSubscription"
	interval := 5 * time.Second
	timeout := 10 * time.Second

	hc := NewHealthChecker(clients, entity, subscription, interval, &HealthCheckerOptions{
		HealthCheckTimeout: timeout,
	})
	if hc.options.HealthCheckTimeout != interval {
		t.Errorf("failed to set HealthCheckTimeout, expected: %s, actual: %s", interval, hc.options.HealthCheckTimeout)
	}

	timeout = 1 * time.Second
	hc = NewHealthChecker(clients, entity, subscription, interval, &HealthCheckerOptions{
		HealthCheckTimeout: timeout,
	})
	if hc.options.HealthCheckTimeout != timeout {
		t.Errorf("failed to set HealthCheckTimeout, expected: %s, actual: %s", timeout, hc.options.HealthCheckTimeout)
	}
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
	interval := 20 * time.Millisecond

	hc := NewHealthChecker(clients, "", "", interval, &HealthCheckerOptions{
		HealthCheckTimeout: 5 * time.Millisecond,
	})
	a.Equal(5*time.Millisecond, hc.options.HealthCheckTimeout)

	for _, tc := range []testCase{
		{entity: "testQueue", subscription: ""},
		{entity: "testTopic", subscription: "testSubscription"},
	} {
		hc.entity = tc.entity
		hc.subscription = tc.subscription
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Millisecond)
		defer cancel()
		hc.StartSenderPeriodicHealthCheck(ctx)
		hc.StartReceiverPeriodicHealthCheck(ctx)

		time.Sleep(100 * time.Millisecond)

		for i := 0; i < numClients; i++ {
			namespaceName := fmt.Sprintf("myservicebus-%d.servicebus.windows.net", i)
			informer := sender.NewInformer()
			failureCount, err := informer.GetHealthCheckFailureCount(namespaceName, tc.entity)
			a.NoError(err)
			a.Equal(5, int(failureCount), "namespace: %s, entity: %s", namespaceName, tc.entity)

			processorInformer := processor.NewInformer()
			processorFailureCount, err := processorInformer.GetHealthCheckFailureCount(namespaceName, tc.entity, tc.subscription)
			a.NoError(err)
			a.Equal(5, int(processorFailureCount), "namespace: %s, entity: %s, subscription: %s", namespaceName, tc.entity, tc.subscription)
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
	subscription := ""
	interval := 20 * time.Millisecond

	hc := NewHealthChecker(clients, entity, subscription, interval, nil)
	a.Equal(interval, hc.options.HealthCheckTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Millisecond)
	defer cancel()
	fakeHealthCheck := func(ctx context.Context, namespace string, client *azservicebus.Client) error {
		sender.Metric.IncHealthCheckSuccessCount(namespace, hc.entity)
		processor.Metric.IncHealthCheckSuccessCount(namespace, hc.entity, hc.subscription)
		return nil
	}
	for namespace, client := range clients {
		go func(namespace string, client *azservicebus.Client) {
			go hc.periodicHealthCheck(ctx, fakeHealthCheck, namespace, client)
		}(namespace, client)
	}

	time.Sleep(100 * time.Millisecond)
	for i := 0; i < numClients; i++ {
		namespaceName := fmt.Sprintf("myservicebus-%d.servicebus.windows.net", i)
		informer := sender.NewInformer()
		successCount, err := informer.GetHealthCheckSuccessCount(namespaceName, entity)
		a.NoError(err)
		a.Equal(5, int(successCount))

		processorInformer := processor.NewInformer()
		processorSuccessCount, err := processorInformer.GetHealthCheckSuccessCount(namespaceName, entity, subscription)
		a.NoError(err)
		a.Equal(5, int(processorSuccessCount))
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
