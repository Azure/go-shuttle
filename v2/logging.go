package shuttle

import (
	"io"
	"log/slog"
)

// logging is disabled by default
var log = slog.New(slog.NewTextHandler(io.Discard, nil))

func getLogger() *slog.Logger {
	return log
}

// SetLogHandler allows to set a custom slog.Handler to be used by the go-shuttle logger.
// If handler is nil, the default slog logger will be used.
func SetLogHandler(handler slog.Handler) {
	if handler == nil {
		log = slog.Default()
	} else {
		log = slog.New(handler)
	}
}
