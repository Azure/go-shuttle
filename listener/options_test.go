package listener

import (
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/Azure/go-autorest/autorest/azure"
)

func TestWithEnvironmentName(t *testing.T) {
	v := assert.New(t)
	expectedEnv := azure.Environment{
		Name:                     "test",
		ServiceBusEndpointSuffix: "testsuffix.com",
	}
	azure.SetEnvironment("test", expectedEnv)
	listener, err := New(WithEnvironmentName("test"))
	v.NoError(err)
	v.Equal(expectedEnv, listener.namespace.Environment)
	v.Equal(expectedEnv.ServiceBusEndpointSuffix, listener.namespace.Suffix)
}
