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

func getLogger(_ context.Context) shuttle.Logger {
	return &testLogger{}
}

func TestSetLoggerFunc(t *testing.T) {
	shuttle.SetLoggerFunc(func(ctx context.Context) shuttle.Logger {
		return getLogger(ctx)
	})
}
