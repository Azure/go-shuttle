package shuttle_test

import (
	"context"
	"testing"

	"github.com/Azure/go-shuttle/v2"
)

type testLogger struct{}

func (t testLogger) Info(s string) {
	//TODO implement me
}

func (t testLogger) Warn(s string) {
	//TODO implement me
}

func (t testLogger) Error(s string) {
	//TODO implement me
}

func getLogger(ctx context.Context) shuttle.Logger {
	return &testLogger{}
}

func TestSetGetLoggerFunc(t *testing.T) {
	shuttle.SetGetLoggerFunc(func(ctx context.Context) shuttle.Logger {
		return getLogger(ctx)
	})
}
