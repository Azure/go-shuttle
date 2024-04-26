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

func SetSlogHandler(handler slog.Handler) {
	if handler != nil {
		log = slog.New(handler)
	}
}
