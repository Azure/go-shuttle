package publisher

import (
	"testing"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/stretchr/testify/assert"

	"github.com/Azure/go-autorest/autorest/azure"
)

func TestWithEnvironmentName(t *testing.T) {
	v := assert.New(t)
	pub := &Publisher{namespace: &servicebus.Namespace{}}
	expectedEnv := azure.Environment{
		Name:                     "test",
		ServiceBusEndpointSuffix: "testsuffix.com",
	}
	azure.SetEnvironment("test", expectedEnv)
	err := WithEnvironmentName("test")(pub)
	v.NoError(err)
	v.Equal(expectedEnv, pub.namespace.Environment)
	v.Equal(expectedEnv.ServiceBusEndpointSuffix, pub.namespace.Suffix)
}
