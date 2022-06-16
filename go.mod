module github.com/Azure/go-shuttle

go 1.14

require (
	github.com/Azure/azure-amqp-common-go/v3 v3.2.0
	github.com/Azure/azure-service-bus-go v0.11.1
	github.com/Azure/go-amqp v0.15.0
	github.com/Azure/go-autorest/autorest v0.11.21
	github.com/Azure/go-autorest/autorest/adal v0.9.16
	github.com/HdrHistogram/hdrhistogram-go v1.0.1 // indirect
	github.com/devigned/tab v0.1.1
	github.com/devigned/tab/opentracing v0.1.3
	github.com/gin-gonic/gin v1.7.1 // indirect
	github.com/gogo/protobuf v1.1.1
	github.com/joho/godotenv v1.3.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.11.0
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/client_model v0.2.0
	github.com/stretchr/testify v1.7.0
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/uber/jaeger-lib v2.4.0+incompatible
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
	google.golang.org/protobuf v1.26.0-rc.1
	nhooyr.io/websocket v1.8.7 // indirect
)

//replace (
//	github.com/Azure/azure-amqp-common-go/v3 => ../azure-amqp-common-go
//)
//	github.com/Azure/azure-service-bus-go => ../azure-service-bus-go
//	github.com/Azure/go-amqp => ../go-amqp
//)
