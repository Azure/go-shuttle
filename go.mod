module github.com/Azure/go-shuttle

go 1.14

require (
	github.com/Azure/azure-amqp-common-go/v3 v3.0.1
	github.com/Azure/azure-service-bus-go v0.10.6
	github.com/Azure/go-autorest/autorest v0.11.6
	github.com/Azure/go-autorest/autorest/adal v0.9.4
	github.com/HdrHistogram/hdrhistogram-go v1.0.1 // indirect
	github.com/devigned/tab v0.1.1
	github.com/devigned/tab/opentracing v0.1.1
	github.com/joho/godotenv v1.3.0
	github.com/onsi/gomega v1.10.2
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.6.1
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/uber/jaeger-lib v2.4.0+incompatible
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/automaxprocs v1.3.0
	golang.org/x/crypto v0.0.0-20200728195943-123391ffb6de
)

replace github.com/Azure/go-amqp => github.com/serbrech/go-amqp v0.13.2-0.20210129001558-f9529f4631b8
