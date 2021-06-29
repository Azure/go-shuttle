package listener

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-shuttle/internal/aad"
	sbinternal "github.com/Azure/go-shuttle/internal/servicebus"
	"github.com/Azure/go-shuttle/tracing/propagation"
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

func WithTracePropagation(traceType propagation.TraceType) Option {
	return func(l *Listener) error {
		l.traceType = traceType
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
		return servicebus.NamespaceWithConnectionString(connStr)(l.namespace)
	}
}

// WithEnvironmentName configures the azure environment used to connect to Servicebus. The environment value used is
// then provided by Azure/go-autorest.
// ref: https://github.com/Azure/go-autorest/blob/c7f947c0610de1bc279f76e6d453353f95cd1bfa/autorest/azure/environments.go#L34
func WithEnvironmentName(environmentName string) ManagementOption {
	return func(l *Listener) error {
		if environmentName == "" {
			return errors.New("cannot use empty environment name")
		}
		return servicebus.NamespaceWithAzureEnvironment(l.namespace.Name, environmentName)(l.namespace)
	}
}

// WithManagedIdentityClientID configures a listener with the attached managed identity and the Service bus resource name
func WithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID string) ManagementOption {
	return func(l *Listener) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		return sbinternal.NamespaceWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID)(l.namespace)
	}
}

// WithToken configures a listener with a AAD token
func WithToken(serviceBusNamespaceName string, spt *adal.ServicePrincipalToken) ManagementOption {
	return func(l *Listener) error {
		if spt == nil {
			return errors.New("cannot provide a nil token")
		}
		return sbinternal.NamespaceWithTokenProvider(serviceBusNamespaceName, aad.AsJWTTokenProvider(spt))(l.namespace)
	}
}

// WithManagedIdentityResourceID configures a listener with the attached managed identity and the Service bus resource name
func WithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) ManagementOption {
	return func(l *Listener) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace provided")
		}
		return sbinternal.NamespaceWithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID)(l.namespace)
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

// WithTypeFilter will subscribe to event of the go type provided.
// It uses the `type` property automatically to messages published via go-shuttle.
func WithTypeFilter(event interface{}) ManagementOption {
	typeName := getTypeName(event)
	return WithFilterDescriber(fmt.Sprintf("tf_%s", typeName), servicebus.SQLFilter{Expression: fmt.Sprintf("type LIKE '%s'", typeName)})
}

func getTypeName(obj interface{}) string {
	valueOf := reflect.ValueOf(obj)
	if valueOf.Type().Kind() == reflect.Ptr {
		return reflect.Indirect(valueOf).Type().Name()
	}
	return valueOf.Type().Name()
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

// WithSubscriptionLockDuration allows listeners to control LockDuration. Passing zeros leaves it up to Service bus defaults
func WithSubscriptionLockDuration(lock time.Duration) ManagementOption {
	return func(l *Listener) error {
		if lock > sbinternal.LockDuration {
			// working on getting service bus to enforce this. Hangs if you go higher. https://github.com/Azure/azure-service-bus-go/pull/202
			return fmt.Errorf("lock duration must be <= to %v", sbinternal.LockDuration)
		}
		if lock < time.Duration(0) {
			return fmt.Errorf("lock duration must be positive")
		}
		l.lockDuration = lock
		return nil
	}
}

// WithSubscriptionMaxDeliveryCount allows listeners to control MaxDeliveryCount. Passing zeros leaves it up to Service bus defaults
func WithSubscriptionMaxDeliveryCount(maxDelivery int32) ManagementOption {
	return func(l *Listener) error {
		if maxDelivery < 0 {
			return fmt.Errorf("max Deliveries must be positive")
		}
		l.maxDeliveryCount = maxDelivery
		return nil
	}
}

// WithPrefetchCount the receiver to quietly acquires more messages, up to the PrefetchCount limit. A single Receive call to the ServiceBus api
// therefore acquires several messages for immediate consumption that is returned as soon as available.
// Please be aware of the consequences : https://docs.microsoft.com/en-us/azure/service-bus-messaging/service-bus-prefetch#if-it-is-faster-why-is-prefetch-not-the-default-option
func WithPrefetchCount(prefetch uint32) Option {
	return func(l *Listener) error {
		if prefetch < 1 {
			return fmt.Errorf("prefetch count value cannot be less than 1")
		}
		if prefetch >= 1 {
			l.prefetchCount = &prefetch
		}
		return nil
	}
}

func WithMaxConcurrency(concurrency int) Option {
	return func(l *Listener) error {
		if concurrency < 0 {
			return fmt.Errorf("concurrency must be greater than 0")
		}
		l.maxConcurrency = &concurrency
		return nil
	}
}
