package v2

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

// DefaultJSONMarshaller is the default marshaller for JSON messages
type DefaultJSONMarshaller struct {
}

// DefaultProtoMarshaller is the default marshaller for protobuf messages
type DefaultProtoMarshaller struct {
}

var _ Marshaller = &DefaultJSONMarshaller{}
var _ Marshaller = &DefaultProtoMarshaller{}

// Marshal marshals the user-input struct into a JSON string and returns a new message with the JSON string as the body
func (j *DefaultJSONMarshaller) Marshal(mb MessageBody) (*azservicebus.Message, error) {
	JSONContentType := j.ContentType()
	str, err := json.Marshal(mb)
	if err != nil {
		return nil, err
	}

	return &azservicebus.Message{Body: str, ContentType: &JSONContentType}, nil
}

// Unmarshal unmarshals the message body from a JSON string into the user-input struct
func (j *DefaultJSONMarshaller) Unmarshal(msg *azservicebus.Message, mb MessageBody) error {
	return json.Unmarshal(msg.Body, mb)
}

// ContentType returns the content type for the JSON marshaller
func (j *DefaultJSONMarshaller) ContentType() string {
	return JsonContentType
}

// Marshal marshals the user-input struct into a protobuf message and returns a new ServiceBus message with the protofbuf message as the body
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

// Unmarshal unmarshalls the protobuf message from the ServiceBus message into the user-input struct
func (p *DefaultProtoMarshaller) Unmarshal(msg *azservicebus.Message, mb MessageBody) error {
	castedMb, ok := mb.(proto.Message)
	if !ok {
		return fmt.Errorf("message body must be a protobuf message")
	}
	return proto.Unmarshal(msg.Body, castedMb)
}

// ContentType returns teh contentType for the protobuf marshaller
func (p *DefaultProtoMarshaller) ContentType() string {
	return protobufContentType
}
