go 1.21
toolchain go1.22.5

module github.com/Azure/go-shuttle/v2

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.14.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.7.0
	github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus v1.7.1
	github.com/devigned/tab v0.1.1
	github.com/onsi/gomega v1.34.2
	github.com/prometheus/client_golang v1.20.2
	github.com/prometheus/client_model v0.6.1
	github.com/stretchr/testify v1.9.0
	go.opentelemetry.io/otel v1.29.0
	go.opentelemetry.io/otel/sdk v1.29.0
	go.opentelemetry.io/otel/trace v1.29.0
	google.golang.org/protobuf v1.34.2
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.10.0 // indirect
	github.com/Azure/go-amqp v1.0.5 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.2.2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/common v0.55.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	go.opentelemetry.io/otel/metric v1.29.0 // indirect
	golang.org/x/crypto v0.26.0 // indirect
	golang.org/x/net v0.28.0 // indirect
	golang.org/x/sys v0.24.0 // indirect
	golang.org/x/text v0.17.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

retract v2.4.0
