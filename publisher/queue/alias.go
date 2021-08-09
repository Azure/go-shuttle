package queue

import (
	"context"
	"time"

	"github.com/Azure/go-autorest/autorest/adal"
	queue "github.com/Azure/go-shuttle/queue/publisher"
)

// Deprecated: use queue package
type Publisher = queue.Publisher

// Deprecated: use queue package
type ManagementOption = queue.ManagementOption

// Deprecated: use queue package
type Option = queue.Option

// Deprecated: use queue package
func New(ctx context.Context, topicName string, opts ...ManagementOption) (*Publisher, error) {
	return New(ctx, topicName, opts...)
}

// Deprecated: use queue package
func WithConnectionString(connStr string) ManagementOption {
	return WithConnectionString(connStr)
}

// Deprecated: use queue package
func WithEnvironmentName(environmentName string) ManagementOption {
	return WithEnvironmentName(environmentName)
}

// Deprecated: use queue package
func WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) ManagementOption {
	return WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID)
}

// Deprecated: use queue package
func WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID string) ManagementOption {
	return WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID)
}

// Deprecated: use queue package
func WithToken(serviceBusNamespaceName string, spt *adal.ServicePrincipalToken) ManagementOption {
	return WithToken(serviceBusNamespaceName, spt)
}

// Deprecated: use queue package
func SetDefaultHeader(headerName, msgKey string) ManagementOption {
	return SetDefaultHeader(headerName, msgKey)
}

// Deprecated: use queue package
func WithDuplicateDetection(window *time.Duration) ManagementOption {
	return WithDuplicateDetection(window)
}

// Deprecated: use queue package
func SetMessageDelay(delay time.Duration) Option {
	return SetMessageDelay(delay)
}

// Deprecated: use queue package
func SetMessageID(messageID string) Option {
	return SetMessageID(messageID)
}

// Deprecated: use queue package
func SetCorrelationID(correlationID string) Option {
	return SetCorrelationID(correlationID)
}
