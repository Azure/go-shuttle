package v2

import (
	"testing"
)

type ContosoCreateUserRequest struct {
	FirstName string
	LastName  string
	Email     string
}

var testStruct = &ContosoCreateUserRequest{FirstName: "John", LastName: "Doe", Email: "johndoe@contoso.com"}

func Test_JSONMarshaller(t *testing.T) {
	testMarshaller := DefaultJSONMarshaller{}
	msg, err := testMarshaller.Marshal(testStruct)
	if err != nil {
		t.Errorf("Unexpected error in marshaller test: %s", err)
	}

	jsonString := `{"FirstName":"John","LastName":"Doe","Email":"johndoe@contoso.com"}`
	if string(msg.Body) != jsonString {
		t.Errorf("for json string expected %s, got %s", jsonString, string(msg.Body))
	}
	if *msg.ContentType != "application/json" {
		t.Errorf("for contenttype expected \"application/json\", got %s", *msg.ContentType)
	}

	var unmarshalledStruct = &ContosoCreateUserRequest{}
	err = testMarshaller.Unmarshal(msg, unmarshalledStruct)
	if err != nil {
		t.Errorf("Unexpected error in unmarshaller: %s", err)
	}

	if !equalStructs(testStruct, unmarshalledStruct) {
		t.Errorf("for unmarshalled struct expected %s, got %s", testStruct, unmarshalledStruct)
	}
}

func equalStructs(expected, actual *ContosoCreateUserRequest) bool {
	return expected.FirstName == actual.FirstName && expected.LastName == actual.LastName && expected.Email == actual.Email
}
