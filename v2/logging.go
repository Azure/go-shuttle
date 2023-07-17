package shuttle

import (
	"context"
	"fmt"
	"os"
	"time"
)

type Logger interface {
	Info(s string)
	Warn(s string)
	Error(s string)
}

// SetLoggerFunc sets the function to be used to acquire a logger when go-shuttle logs.
func SetLoggerFunc(fn func(ctx context.Context) Logger) {
	getLogger = fn
}

var getLogger = func(_ context.Context) Logger { return &printLogger{} }

type printLogger struct{}

func (l *printLogger) Info(s string) {
	fmt.Println(append(append([]any{}, "INFO - ", time.Now().UTC(), " - "), s)...)
}

func (l *printLogger) Warn(s string) {
	fmt.Println(append(append([]any{}, "WARN - ", time.Now().UTC(), " - "), s)...)
}

func (l *printLogger) Error(s string) {
	fmt.Println(append(append([]any{}, "ERROR - ", time.Now().UTC(), " - "), s)...)
}

func log(ctx context.Context, a ...any) {
	if os.Getenv("GOSHUTTLE_LOG") == "ALL" {
		getLogger(ctx).Info(fmt.Sprint(append(append([]any{}, time.Now().UTC(), " - "), a...)...))
	}
}
