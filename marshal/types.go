package marshal

import (
	"encoding/json"
	"fmt"

	"github.com/gogo/protobuf/proto"

	"github.com/Azure/go-shuttle/common"
)

const ProtobufContentType = "application/x-protobuf"
const JSONContentType = "application/json"

var ProtobufMarshaller = &ProtoMarshaller{}
var JSONMarshaller = &JsonMarshaller{}

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

var _ common.Marshaller = &ProtoMarshaller{}

type JsonMarshaller struct {
}

func (p *JsonMarshaller) ContentType() string {
	return JSONContentType
}

func (p *JsonMarshaller) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

var _ common.Marshaller = &JsonMarshaller{}
