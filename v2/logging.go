package shuttle

import (
	"context"
	"log/slog"
)

type Logger interface {
	Info(s string)
	Warn(s string)
	Error(s string)
}

var getLogger = func(ctx context.Context) Logger {
	return &contextLogger{ctx: ctx, logger: slog.Default()}
}

// Deprecated: Use SetLogHandler instead to adapt slog.
// SetLoggerFunc sets the function to be used to acquire a logger when go-shuttle logs.
func SetLoggerFunc(fn func(ctx context.Context) Logger) {
	getLogger = fn
}

// SetLogHandler allows to set a custom slog.Handler to be used by the go-shuttle logger.
// If handler is nil, the default slog handler will be used.
func SetLogHandler(handler slog.Handler) {
	if handler == nil {
		handler = slog.Default().Handler()
	}
	getLogger = func(ctx context.Context) Logger {
		return &contextLogger{ctx: ctx, logger: slog.New(handler)}
	}
}

type contextLogger struct {
	ctx    context.Context
	logger *slog.Logger
}

func (l *contextLogger) Info(s string) {
	l.logger.InfoContext(l.ctx, s)
}

func (l *contextLogger) Warn(s string) {
	l.logger.WarnContext(l.ctx, s)
}

func (l *contextLogger) Error(s string) {
	l.logger.ErrorContext(l.ctx, s)
}
