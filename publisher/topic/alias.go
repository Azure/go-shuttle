package topic

import (
	"context"
	topic "github.com/Azure/go-shuttle/topic/publisher"
	"time"

	"github.com/Azure/go-autorest/autorest/adal"

)

// Deprecated: use topic package
type Publisher = topic.Publisher

// Deprecated: use topic package
type ManagementOption = topic.ManagementOption

// Deprecated: use topic package
type Option = topic.Option

// Deprecated: use topic package
func New(ctx context.Context, topicName string, opts ...ManagementOption) (*Publisher, error) {
	return New(ctx, topicName, opts...)
}

// Deprecated: use topic package
func WithConnectionString(connStr string) ManagementOption {
	return WithConnectionString(connStr)
}

// Deprecated: use topic package
func WithEnvironmentName(environmentName string) ManagementOption {
	return WithEnvironmentName(environmentName)
}

// Deprecated: use topic package
func WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) ManagementOption {
	return WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID)
}

// Deprecated: use topic package
func WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID string) ManagementOption {
	return WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID)
}

// Deprecated: use topic package
func WithToken(serviceBusNamespaceName string, spt *adal.ServicePrincipalToken) ManagementOption {
	return WithToken(serviceBusNamespaceName, spt)
}

// Deprecated: use topic package
func SetDefaultHeader(headerName, msgKey string) ManagementOption {
	return SetDefaultHeader(headerName, msgKey)
}

// Deprecated: use topic package
func WithDuplicateDetection(window *time.Duration) ManagementOption {
	return WithDuplicateDetection(window)
}

// Deprecated: use topic package
func SetMessageDelay(delay time.Duration) Option {
	return SetMessageDelay(delay)
}

// Deprecated: use topic package
func SetMessageID(messageID string) Option {
	return SetMessageID(messageID)
}

// Deprecated: use topic package
func SetCorrelationID(correlationID string) Option {
	return SetCorrelationID(correlationID)
}