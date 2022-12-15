package sender

import (
	"encoding/json"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"google.golang.org/protobuf/proto"
)

type Marshaller interface {
	Marshal(mb MessageBody) (*azservicebus.Message, error)
	Unmarshal(msg *azservicebus.Message, mb MessageBody) error
	ContentType() string
}

const JsonContentType = "application/json"
const protobufContentType = "application/x-protobuf"

type DefaultJSONMarshaller struct {
}

type DefaultProtoMarshaller struct {
}

var _ Marshaller = &DefaultJSONMarshaller{}
var _ Marshaller = &DefaultProtoMarshaller{}

func (j *DefaultJSONMarshaller) Marshal(mb MessageBody) (*azservicebus.Message, error) {
	JSONContentType := j.ContentType()
	str, err := json.Marshal(mb)
	if err != nil {
		return nil, err
	}

	return &azservicebus.Message{Body: str, ContentType: &JSONContentType}, nil
}

func (j *DefaultJSONMarshaller) Unmarshal(msg *azservicebus.Message, mb MessageBody) error {
	return json.Unmarshal(msg.Body, mb)
}

func (j *DefaultJSONMarshaller) ContentType() string {
	return JsonContentType
}

func (p *DefaultProtoMarshaller) Marshal(mb MessageBody) (*azservicebus.Message, error) {
	protoContentType := p.ContentType()
	message, ok := mb.(proto.Message)

	if !ok {
		return nil, fmt.Errorf("message must be a protobuf message")
	}
	data, err := proto.Marshal(message)
	if err != nil {
		return nil, err
	}
	msg := &azservicebus.Message{Body: data, ContentType: &protoContentType}

	return msg, nil
}

func (p *DefaultProtoMarshaller) Unmarshal(msg *azservicebus.Message, mb MessageBody) error {
	castedMb, ok := mb.(proto.Message)
	if !ok {
		return fmt.Errorf("message body must be a protobuf message")
	}
	return proto.Unmarshal(msg.Body, castedMb)
}

func (p *DefaultProtoMarshaller) ContentType() string {
	return protobufContentType
}
