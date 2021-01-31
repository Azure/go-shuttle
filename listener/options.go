package listener

import (
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-shuttle/internal/aad"
	sbinternal "github.com/Azure/go-shuttle/internal/servicebus"
)

// Option provides structure for configuring when starting to listen to a specified topic
type Option func(l *Listener) error

func WithMessageLockAutoRenewal(interval time.Duration) Option {
	return func(l *Listener) error {
		if interval < time.Duration(0) {
			return fmt.Errorf("renewal interval must be positive")
		}
		l.lockRenewalInterval = &interval
		return nil
	}
}

// ManagementOption provides structure for configuring a new Listener
type ManagementOption func(l *Listener) error

// WithConnectionString configures a listener with the information provided in a Service Bus connection string
func WithConnectionString(connStr string) ManagementOption {
	return func(l *Listener) error {
		if connStr == "" {
			return errors.New("no Service Bus connection string provided")
		}
		ns, err := servicebus.NewNamespace(servicebus.NamespaceWithConnectionString(connStr))
		if err != nil {
			return err
		}
		l.namespace = ns
		return nil
	}
}

// WithManagedIdentityClientID configures a listener with the attached managed identity and the Service bus resource name
func WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID string) ManagementOption {
	return func(l *Listener) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		ns, err := servicebus.NewNamespace(sbinternal.NamespaceWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID))
		if err != nil {
			return err
		}
		l.namespace = ns
		return nil
	}
}

// WithToken configures a listener with a AAD token
func WithToken(serviceBusNamespaceName string, spt *adal.ServicePrincipalToken) ManagementOption {
	return func(l *Listener) error {
		if spt == nil {
			return errors.New("cannot provide a nil token")
		}
		ns, err := servicebus.NewNamespace(sbinternal.NamespaceWithTokenProvider(serviceBusNamespaceName, aad.AsJWTTokenProvider(spt)))
		if err != nil {
			return err
		}
		l.namespace = ns
		return nil
	}
}

// WithManagedIdentityResourceID configures a listener with the attached managed identity and the Service bus resource name
func WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) ManagementOption {
	return func(l *Listener) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		ns, err := servicebus.NewNamespace(sbinternal.NamespaceWithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID))
		if err != nil {
			return err
		}
		l.namespace = ns
		return nil
	}
}

// WithSubscriptionName configures the subscription name of the subscription to listen to
func WithSubscriptionName(name string) ManagementOption {
	return func(l *Listener) error {
		l.subscriptionName = name
		return nil
	}
}

// WithFilterDescriber configures the filters on the subscription
func WithFilterDescriber(filterName string, filter servicebus.FilterDescriber) ManagementOption {
	return func(l *Listener) error {
		if len(filterName) == 0 || filter == nil {
			return errors.New("filter name or filter cannot be zero value")
		}
		l.filterDefinitions = append(l.filterDefinitions, &filterDefinition{filterName, filter})
		return nil
	}
}

// WithSubscriptionDetails allows listeners to control subscription details for longer lived operations.
// If you using RetryLater you probably want this. Passing zeros leaves it up to Service bus defaults
func WithSubscriptionDetails(lock time.Duration, maxDelivery int32) ManagementOption {
	return func(l *Listener) error {
		if lock > sbinternal.LockDuration {
			// working on getting service bus to enforce this. Hangs if you go higher. https://github.com/Azure/azure-service-bus-go/pull/202
			return fmt.Errorf("lock duration must be <= to %v", sbinternal.LockDuration)
		}
		if lock < time.Duration(0) {
			return fmt.Errorf("lock duration must be positive")
		}
		l.lockDuration = lock
		if maxDelivery < 0 {
			return fmt.Errorf("max Deliveries must be positive")
		}
		l.maxDeliveryCount = maxDelivery
		return nil
	}
}
