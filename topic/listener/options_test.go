package listener

import (
	"testing"

	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/stretchr/testify/assert"
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
	v.Equal(expectedEnv, listener.Namespace().Environment)
	v.Equal(expectedEnv.ServiceBusEndpointSuffix, listener.Namespace().Suffix)
}
