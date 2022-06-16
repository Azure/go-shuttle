package marshal_test

import (
	"testing"

	"github.com/Azure/go-shuttle/marshal"
)

type customMarshaller struct {
	marshalCalled   bool
	unmarshalCalled bool
	contentType     string
}

func (c *customMarshaller) Marshal(v interface{}) ([]byte, error) {
	c.marshalCalled = true
	return nil, nil
}

func (c *customMarshaller) Unmarshal(data []byte, v interface{}) error {
	c.unmarshalCalled = true
	return nil
}

func (c *customMarshaller) ContentType() string {
	return "test"
}

func TestRegister(t *testing.T) {
	marshaler := &customMarshaller{}
	marshal.RegisterMarshaller(marshaler)
	if marshal.DefaultMarshallerRegistry["test"] != marshaler {
		t.Fail()
	}
}

func TestPreRegisteredMarshallers(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		marshaller  marshal.Marshaller
	}{
		{"empty content type", "", marshal.JSONMarshaller},
		{"json content type", marshal.JSONMarshaller.ContentType(), marshal.JSONMarshaller},
		{"proto content type", marshal.ProtobufMarshaller.ContentType(), marshal.ProtobufMarshaller},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(tt *testing.T) {
			tt.Parallel()
			m, ok := marshal.DefaultMarshallerRegistry[tc.contentType]
			if !ok {
				tt.Fail()
			}
			if m != tc.marshaller {
				tt.Fail()
			}
		})
	}
}

func TestRegisterMarshaller(t *testing.T) {

}
