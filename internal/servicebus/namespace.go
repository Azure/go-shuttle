package servicebus

import (
	"errors"

	"github.com/Azure/azure-amqp-common-go/v3/auth"
	servicebussdk "github.com/Azure/azure-service-bus-go"

	"github.com/keikumata/azure-pub-sub/internal/aad"
)

const (
	// TODO:take this value from config or environment
	serviceBusResourceURI = "https://servicebus.azure.net/"
)

var errorOption func(h *servicebussdk.Namespace) error
func newErrorOption(err error) servicebussdk.NamespaceOption {
	return func(ns *servicebussdk.Namespace) error { return err }
}

// NamespaceWithManagedIdentity is a custom NamespaceOption to instantiate a Service Bus namespace client with
// managed identity resource id
func NamespaceWithManagedIdentityResourceID(serviceBusNamespaceName, managedIdentityResourceID string) servicebussdk.NamespaceOption {
		provider, err := aad.NewJWTProvider(
			aad.JWTProviderWithManagedIdentityResourceID(managedIdentityResourceID, ""),
			aad.JWTProviderWithResourceURI(serviceBusResourceURI),
		)
		if err != nil {
			// TODO: make this fail at creation of the option, instead of runtime.
			return newErrorOption(err)
		}
		return NamespaceWithTokenProvider(serviceBusNamespaceName, provider)
}

// NamespaceWithManagedIdentityClientID is a custom NamespaceOption to instantiate a Service Bus namespace client with
// managed identity client id
func NamespaceWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID string) servicebussdk.NamespaceOption {
	provider, err := aad.NewJWTProvider(
		aad.JWTProviderWithManagedIdentityClientID(managedIdentityClientID, ""),
		aad.JWTProviderWithResourceURI(serviceBusResourceURI),
	)
	if err != nil {
		// TODO: make this fail at creation of the option, instead of runtime.
		return newErrorOption(err)
	}
	return NamespaceWithTokenProvider(serviceBusNamespaceName, provider)
}

func NamespaceWithTokenProvider(serviceBusNamespaceName string, provider auth.TokenProvider) servicebussdk.NamespaceOption {
	return func(ns *servicebussdk.Namespace) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace name provided")
		}

		ns.TokenProvider = provider
		ns.Name = serviceBusNamespaceName
		return nil
	}
}

// NamespaceWithManagedIdentity is deprecated. use NamespaceWithManagedIdentityClientID or NamespaceWithManagedIdentityResourceID
// managed identity
// Deprecated
func NamespaceWithManagedIdentity(serviceBusNamespaceName, managedIdentityClientID string) servicebussdk.NamespaceOption {
	return NamespaceWithManagedIdentityClientID(serviceBusNamespaceName, managedIdentityClientID)
}