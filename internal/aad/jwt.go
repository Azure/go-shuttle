package aad

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"strconv"
	"time"

	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"golang.org/x/crypto/pkcs12"

	"github.com/Azure/azure-amqp-common-go/v3/auth"
)

const (
	eventhubResourceURI = "https://eventhubs.azure.net/"
)

type (
	// TokenProviderConfiguration provides configuration parameters for building JWT AAD providers
	TokenProviderConfiguration struct {
		TenantID            string
		ClientID            string
		ResourceID          string
		ClientSecret        string
		CertificatePath     string
		CertificatePassword string
		ResourceURI         string
		aadToken            *adal.ServicePrincipalToken
		Env                 *azure.Environment
	}

	// TokenProvider provides cbs.TokenProvider functionality for Azure Active Directory JWTs
	TokenProvider struct {
		tokenProvider *adal.ServicePrincipalToken
	}

	// JWTProviderOption provides configuration options for constructing AAD Token Providers
	JWTProviderOption func(provider *TokenProviderConfiguration) error
)

// JWTProviderWithAzureEnvironment configures the token provider to use a specific Azure Environment
func JWTProviderWithAzureEnvironment(env *azure.Environment) JWTProviderOption {
	return func(config *TokenProviderConfiguration) error {
		config.Env = env
		return nil
	}
}

// JWTProviderWithVars configures the TokenProvider with client credentials
// - attempt to authenticate with a Service Principal via tenantID, clientID and clientSecret
func JWTProviderWithClientCredentials(clientID, clientSecret, tenantID, environment string) JWTProviderOption {
	return func(config *TokenProviderConfiguration) error {
		config.TenantID = tenantID
		config.ClientID = clientID
		config.ClientSecret = clientSecret

		if config.Env == nil {
			env, err := azureEnvFromEnvironment(environment)
			if err != nil {
				return err
			}
			config.Env = env
		}
		return nil
	}
}

// JWTProviderWithClientCertificate configures the TokenProvider with client certificate
// - attempt to authenticate with a Service Principal via tenantID, clientID, certificatePath, and certificatePassword
func JWTProviderWithClientCertificate(clientID, certificatePath, certificatePassword, tenantID, environment string) JWTProviderOption {
	return func(config *TokenProviderConfiguration) error {
		config.TenantID = tenantID
		config.ClientID = clientID
		config.CertificatePath = certificatePath
		config.CertificatePassword = certificatePassword

		if config.Env == nil {
			env, err := azureEnvFromEnvironment(environment)
			if err != nil {
				return err
			}
			config.Env = env
		}
		return nil
	}
}

// JWTProviderWithManagedIdentityResourceID configures the TokenProvider using managed identity
// - attempt to authenticate using managed identity resourceID. Otherwise it will default to the system assigned managed
//   identity
func JWTProviderWithManagedIdentityResourceID(resourceID, environment string) JWTProviderOption {
	return func(config *TokenProviderConfiguration) error {
		config.ResourceID = resourceID

		if config.Env == nil {
			env, err := azureEnvFromEnvironment(environment)
			if err != nil {
				return err
			}
			config.Env = env
		}
		return nil
	}
}

// JWTProviderWithManagedIdentityClientID configures the TokenProvider using managed identity clientID
// - attempt to authenticate using managed identity clientID. Otherwise it will default to the system assigned managed
//   identity
func JWTProviderWithManagedIdentityClientID(clientID, environment string) JWTProviderOption {
	return func(config *TokenProviderConfiguration) error {
		config.ClientID = clientID

		if config.Env == nil {
			env, err := azureEnvFromEnvironment(environment)
			if err != nil {
				return err
			}
			config.Env = env
		}
		return nil
	}
}

// JWTProviderWithManagedIdentity configures the TokenProvider using managed identity
// - attempt to authenticate using managed identity. If clientID is supplied then it will use that
//   as the clientID of the user assigned managed identity. Otherwise it will default to the system assigned managed
//   identity
// Deprecated use JWTProviderWithManagedIdentityClientID
func JWTProviderWithManagedIdentity(clientID, environment string) JWTProviderOption {
	return JWTProviderWithManagedIdentityClientID(clientID, environment)
}

// JWTProviderWithResourceURI configures the token provider to use a specific eventhubResourceURI URI
func JWTProviderWithResourceURI(resourceURI string) JWTProviderOption {
	return func(config *TokenProviderConfiguration) error {
		config.ResourceURI = resourceURI
		return nil
	}
}

// JWTProviderWithAADToken configures the token provider to use a specific Azure Active Directory Service Principal token
func JWTProviderWithAADToken(aadToken *adal.ServicePrincipalToken) JWTProviderOption {
	return func(config *TokenProviderConfiguration) error {
		config.aadToken = aadToken
		return nil
	}
}

// NewJWTProvider builds an Azure Active Directory claims-based security token provider
func NewJWTProvider(opts ...JWTProviderOption) (*TokenProvider, error) {
	config := &TokenProviderConfiguration{
		ResourceURI: eventhubResourceURI,
	}

	for _, opt := range opts {
		err := opt(config)
		if err != nil {
			return nil, err
		}
	}

	if config.aadToken == nil {
		spToken, err := config.NewServicePrincipalToken()
		if err != nil {
			return nil, err
		}
		config.aadToken = spToken
	}
	return &TokenProvider{tokenProvider: config.aadToken}, nil
}

// NewServicePrincipalToken creates a new Azure Active Directory Service Principal token provider
func (c *TokenProviderConfiguration) NewServicePrincipalToken() (*adal.ServicePrincipalToken, error) {
	oauthConfig, err := adal.NewOAuthConfig(c.Env.ActiveDirectoryEndpoint, c.TenantID)
	if err != nil {
		return nil, err
	}

	// 1.Client Credentials
	if c.ClientSecret != "" {
		spToken, err := adal.NewServicePrincipalToken(*oauthConfig, c.ClientID, c.ClientSecret, c.ResourceURI)
		if err != nil {
			return nil, fmt.Errorf("failed to get oauth token from client credentials: %v", err)
		}
		if err := spToken.Refresh(); err != nil {
			return nil, fmt.Errorf("failed to refersh token: %v", spToken)
		}
		return spToken, nil
	}

	// 2. Client Certificate
	if c.CertificatePath != "" {
		certData, err := ioutil.ReadFile(c.CertificatePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read the certificate file (%s): %v", c.CertificatePath, err)
		}
		certificate, rsaPrivateKey, err := decodePkcs12(certData, c.CertificatePassword)
		if err != nil {
			return nil, fmt.Errorf("failed to decode pkcs12 certificate while creating spt: %v", err)
		}
		spToken, err := adal.NewServicePrincipalTokenFromCertificate(*oauthConfig, c.ClientID, certificate, rsaPrivateKey, c.ResourceURI)
		if err != nil {
			return nil, fmt.Errorf("failed to get oauth token from certificate auth: %v", err)
		}
		if err := spToken.Refresh(); err != nil {
			return nil, fmt.Errorf("failed to refersh token: %v", spToken)
		}
		return spToken, nil
	}

	// 3. By default return MSI
	msiEndpoint, err := adal.GetMSIVMEndpoint()
	if err != nil {
		return nil, err
	}
	spToken, err := c.getTokenProvider(msiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token from MSI: %v", err)
	}
	if err := spToken.Refresh(); err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	return spToken, nil
}

func (c *TokenProviderConfiguration) getTokenProvider(msiEndpoint string) (*adal.ServicePrincipalToken, error) {
	if c.ClientID != "" {
		return adal.NewServicePrincipalTokenFromMSIWithUserAssignedID(msiEndpoint, c.ResourceURI, c.ClientID)
	}
	if c.ResourceID != "" {
		return adal.NewServicePrincipalTokenFromMSIWithIdentityResourceID(msiEndpoint, c.ResourceURI, c.ResourceID)
	}
	return adal.NewServicePrincipalTokenFromMSI(msiEndpoint, c.ResourceURI)
}

// GetToken gets a CBS JWT
func (t *TokenProvider) GetToken(audience string) (*auth.Token, error) {
	token := t.tokenProvider.Token()
	expireTicks, err := strconv.ParseInt(string(token.ExpiresOn), 10, 64)
	if err != nil {
		return nil, err
	}
	expires := time.Unix(expireTicks, 0)

	if expires.Before(time.Now()) {
		if err := t.tokenProvider.Refresh(); err != nil {
			return nil, err
		}
		token = t.tokenProvider.Token()
	}

	return auth.NewToken(auth.CBSTokenTypeJWT, token.AccessToken, string(token.ExpiresOn)), nil
}

func decodePkcs12(pkcs []byte, password string) (*x509.Certificate, *rsa.PrivateKey, error) {
	privateKey, certificate, err := pkcs12.Decode(pkcs, password)
	if err != nil {
		return nil, nil, err
	}

	rsaPrivateKey, isRsaKey := privateKey.(*rsa.PrivateKey)
	if !isRsaKey {
		return nil, nil, fmt.Errorf("PKCS#12 certificate must contain an RSA private key")
	}

	return certificate, rsaPrivateKey, nil
}

func azureEnvFromEnvironment(envName string) (*azure.Environment, error) {
	var env azure.Environment
	if envName == "" {
		env = azure.PublicCloud
	} else {
		var err error
		env, err = azure.EnvironmentFromName(envName)
		if err != nil {
			return nil, err
		}
	}
	return &env, nil
}
