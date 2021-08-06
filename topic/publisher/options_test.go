package publisher

import (
	servicebus "github.com/Azure/azure-service-bus-go"
	"testing"

	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/stretchr/testify/assert"
)

func TestWithEnvironmentName(t *testing.T) {
	v := assert.New(t)
	pub := &Publisher{}
	pub.SetNamespace(&servicebus.Namespace{})
	expectedEnv := azure.Environment{
		Name:                     "test",
		ServiceBusEndpointSuffix: "testsuffix.com",
	}
	azure.SetEnvironment("test", expectedEnv)
	err := WithEnvironmentName("test")(pub)
	v.NoError(err)
	v.Equal(expectedEnv, pub.Namespace().Environment)
	v.Equal(expectedEnv.ServiceBusEndpointSuffix, pub.Namespace().Suffix)
}
