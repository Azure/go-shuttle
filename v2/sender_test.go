package v2

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

func TestHandlers_SetType(t *testing.T) {
	type ContosoCreateUserRequest struct {
		FirstName string
		LastName  string
		Email     string
	}

	testStruct := ContosoCreateUserRequest{}
	expectedContentType := "ContosoCreateUserRequest"
	blankMsg := &azservicebus.Message{}
	handler := SetTypeHandler(testStruct)
	err := handler(context.Background(), blankMsg)
	if err != nil {
		t.Errorf("Unexpected error in set type test: %s", err)
	}
	if *blankMsg.ContentType != expectedContentType {
		t.Errorf("for contenttype expected %s, got %s", expectedContentType, *blankMsg.ContentType)

	}

}

func TestHandlers_SetMessageId(t *testing.T) {
	randId := "testmessageid"

	blankMsg := &azservicebus.Message{}
	handler := SetMessageIdHandler(&randId)
	err := handler(context.Background(), blankMsg)
	if err != nil {
		t.Errorf("Unexpected error in set message id test: %s", err)
	}
	if *blankMsg.MessageID != randId {
		t.Errorf("for message id expected %s, got %s", randId, *blankMsg.MessageID)
	}
}
