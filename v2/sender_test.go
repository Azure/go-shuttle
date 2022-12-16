package v2

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

func ContentVerifyer(expectedContent string, t *testing.T) SendOption {
	return func(ctx context.Context, message *azservicebus.Message) {
		body := message.Body
		if string(body) != expectedContent {
			t.Errorf("expected %s, got %s", expectedContent, body)
		}
	}
}

func ContentTypeVerifyer(expectedContentType string, t *testing.T) SendOption {
	return func(ctx context.Context, message *azservicebus.Message) {
		contentType := *message.ContentType
		if contentType != expectedContentType {
			t.Errorf("expected %s, got %s", expectedContentType, contentType)
		}
	}
}

func MessageIdVerifyer(expectedContent string, t *testing.T) SendOption {
	return func(ctx context.Context, message *azservicebus.Message) {
		body := *message.MessageID
		if body != expectedContent {
			t.Errorf("expected %s, got %s", expectedContent, body)
		}
	}
}

func TestHandlers_SetType(t *testing.T) {
	type ContosoCreateUserRequest struct {
		FirstName string
		LastName  string
		Email     string
	}

	testStruct := ContosoCreateUserRequest{}
	verifyContentTypeHandler := ContentTypeVerifyer("ContosoCreateUserRequest", t)
	blankMsg := &azservicebus.Message{}
	handler := SetTypeHandler(testStruct, verifyContentTypeHandler)
	handler(context.Background(), blankMsg)

}
