package marshal

import (
	"encoding/json"
	"fmt"

	"github.com/gogo/protobuf/proto"
)

const ProtobufContentType = "application/x-protobuf"
const JSONContentType = "application/json"

func init() {
	RegisterMarshaller(JSONMarshaller)
	RegisterMarshaller(ProtobufMarshaller)
	// register json unmarshaller for empty content-type
	DefaultMarshallerRegistry[""] = JSONMarshaller
}

type Marshaller interface {
	Marshal(v interface{}) ([]byte, error)
	Unmarshal(data []byte, v any) error
	ContentType() string
}

var DefaultMarshallerRegistry map[string]Marshaller

func RegisterMarshaller(marshaller Marshaller) {
	DefaultMarshallerRegistry[marshaller.ContentType()] = marshaller
}

var ProtobufMarshaller = &ProtoMarshaller{}
var JSONMarshaller = &JsonMarshaller{}

var _ Marshaller = &ProtoMarshaller{}
var _ Marshaller = &JsonMarshaller{}

type ProtoMarshaller struct {
}

func (p *ProtoMarshaller) ContentType() string {
	return ProtobufContentType
}

func (p *ProtoMarshaller) Marshal(v interface{}) ([]byte, error) {
	message, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("message must be a protobuf Message")
	}
	return proto.Marshal(message)
}

func (p *ProtoMarshaller) Unmarshal(data []byte, v any) error {
	message, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("v must be a protobuf Message")
	}
	return proto.Unmarshal(data, message)
}

type JsonMarshaller struct {
}

func (j *JsonMarshaller) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func (j *JsonMarshaller) ContentType() string {
	return JSONContentType
}

func (j *JsonMarshaller) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
