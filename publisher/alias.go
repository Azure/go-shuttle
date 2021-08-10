package publisher

import (
	"context"
	"time"

	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-shuttle/publisher/topic"
)

// Deprecated: use topic package
type Publisher = topic.Publisher

// Deprecated: use topic package
type ManagementOption = topic.ManagementOption

// Deprecated: use topic package
type Option = topic.Option

// Deprecated: use topic package
func New(ctx context.Context, topicName string, opts ...ManagementOption) (*Publisher, error) {
	return topic.New(ctx, topicName, opts...)
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
func WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) ManagementOption {
	return topic.WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID)
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
func SetDefaultHeader(headerName, msgKey string) ManagementOption {
	return topic.SetDefaultHeader(headerName, msgKey)
}

// Deprecated: use topic package
func WithDuplicateDetection(window *time.Duration) ManagementOption {
	return topic.WithDuplicateDetection(window)
}

// Deprecated: use topic package
func SetMessageDelay(delay time.Duration) Option {
	return topic.SetMessageDelay(delay)
}

// Deprecated: use topic package
func SetMessageID(messageID string) Option {
	return topic.SetMessageID(messageID)
}

// SetCorrelationID sets the SetCorrelationID of the message.
func SetCorrelationID(correlationID string) Option {
	return topic.SetCorrelationID(correlationID)
}