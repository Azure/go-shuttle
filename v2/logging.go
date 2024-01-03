package shuttle

import (
	"context"

	"github.com/sirupsen/logrus"
)

type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Panic(args ...interface{})
}

// SetLoggerFunc sets the function to be used to acquire a logger when go-shuttle logs.
func SetLoggerFunc(fn func(ctx context.Context) Logger) {
	getLoggerFn = fn
}

var getLoggerFn = func(_ context.Context) Logger { return logrus.StandardLogger() }

func getLogger(ctx context.Context) Logger {
	logger := getLoggerFn(ctx)
	if logger == nil { // fallback to standard logger
		logger = logrus.StandardLogger()
	}
	return logger
}
