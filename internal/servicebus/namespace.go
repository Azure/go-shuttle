package servicebus

import (
	"errors"

	servicebussdk "github.com/Azure/azure-service-bus-go"

	"github.com/keikumata/azure-pub-sub/internal/aad"
)

const (
	serviceBusResourceURI = "https://servicebus.azure.net/"
)

// NamespaceWithManagedIdentity is a custom NamespaceOption to instantiate a Service Bus namespace client with
// managed identity
func NamespaceWithManagedIdentity(serviceBusNamespaceName, managedIdentityClientID string) servicebussdk.NamespaceOption {
	return func(ns *servicebussdk.Namespace) error {
		if serviceBusNamespaceName == "" {
			return errors.New("no Service Bus namespace name provided")
		}
		provider, err := aad.NewJWTProvider(
			aad.JWTProviderWithManagedIdentity(managedIdentityClientID, ""),
			aad.JWTProviderWithResourceURI(serviceBusResourceURI),
		)
		if err != nil {
			return err
		}

		ns.TokenProvider = provider
		ns.Name = serviceBusNamespaceName
		return nil
	}
}