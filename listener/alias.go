package listener

import (
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-autorest/autorest/adal"
	topic "github.com/Azure/go-shuttle/topic/listener"
)

// Deprecated: use topic package
type Listener = topic.Listener

// Deprecated: use topic package
type Option = topic.Option

// Deprecated: use topic package
type ManagementOption = topic.ManagementOption

// Deprecated: use topic package
func New(opts ...ManagementOption) (*Listener, error) {
	return topic.New(opts...)
}

// Deprecated: use topic package
func WithMessageLockAutoRenewal(interval time.Duration) Option {
	return topic.WithMessageLockAutoRenewal(interval)
}

// Deprecated: use topic package
func WithConnectionString(connStr string) ManagementOption {
	return topic.WithConnectionString(connStr)
}

// Deprecated: use topic package
func WithEnvironmentName(environmentName string) ManagementOption {
	return topic.WithEnvironmentName(environmentName)
}

// Deprecated: use topic package
func WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID string) ManagementOption {
	return topic.WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID)
}

// Deprecated: use topic package
func WithToken(serviceBusNamespaceName string, spt *adal.ServicePrincipalToken) ManagementOption {
	return topic.WithToken(serviceBusNamespaceName, spt)
}

// Deprecated: use topic package
func WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) ManagementOption {
	return topic.WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID)
}

// Deprecated: use topic package
func WithSubscriptionDetails(lock time.Duration, maxDelivery int32) ManagementOption {
	return topic.WithSubscriptionDetails(lock, maxDelivery)
}

// Deprecated: use topic package
func WithLockDuration(lock time.Duration) ManagementOption {
	return topic.WithLockDuration(lock)
}

// Deprecated: use topic package
func WithMaxDeliveryCount(maxDelivery int32) ManagementOption {
	return topic.WithMaxDeliveryCount(maxDelivery)
}

// Deprecated: use topic package
func WithPrefetchCount(prefetch uint32) Option {
	return topic.WithPrefetchCount(prefetch)
}

// Deprecated: use topic package
func WithMaxConcurrency(concurrency int) Option {
	return topic.WithMaxConcurrency(concurrency)
}

// Deprecated: use topic package
func WithSubscriptionName(name string) ManagementOption {
	return topic.WithSubscriptionName(name)
}

// Deprecated: use topic package
func WithFilterDescriber(filterName string, filter servicebus.FilterDescriber) ManagementOption {
	return topic.WithFilterDescriber(filterName, filter)
}

// Deprecated: use topic package
func WithTypeFilter(event interface{}) ManagementOption {
	return topic.WithTypeFilter(event)
}