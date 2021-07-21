package errorhandling

import (
	"fmt"
	"io"
	"syscall"
	"testing"

	"github.com/Azure/go-amqp"
)

type temporaryError struct {
	message string
}

func (te temporaryError) Error() string {
	return te.message
}

func (te temporaryError) Timeout() bool {
	return false
}

func (te temporaryError) Temporary() bool {
	return true
}

func TestIsConnectionDead(t *testing.T) {
	tests := []struct {
		name       string
		givenError error
		want       bool
	}{
		{name: "syscall-timeoutError", givenError: syscall.ETIMEDOUT, want: true},
		{name: "randomError", givenError: fmt.Errorf("random error"), want: false},
		{name: "anyAmqpError", givenError: &amqp.Error{}, want: false},
		{name: "io.EOF", givenError: io.EOF, want: true},
		{name: "AmqpInternalError", givenError: &amqp.Error{
			Condition:   amqp.ErrorInternalError,
			Description: "The service was unable to process the request",
			Info:        nil,
		}, want: true},
		{name: "temporaryError", givenError: temporaryError{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsConnectionDead(tt.givenError); got != tt.want {
				t.Errorf("IsConnectionDead() = %v, want %v", got, tt.want)
			}
		})
	}
}
