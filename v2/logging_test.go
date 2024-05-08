package shuttle

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	. "github.com/onsi/gomega"
)

func TestSetSlogHandler(t *testing.T) {
	g := NewWithT(t)
	g.Expect(func() { getLogger(context.Background()).Info("test") }).ToNot(Panic())
	g.Expect(func() { SetLogHandler(nil) }).ToNot(Panic())

	buf := &bytes.Buffer{}
	SetLogHandler(slog.NewTextHandler(buf, nil))
	getLogger(context.Background()).Info("testInfo")
	g.Expect(buf.String()).To(ContainSubstring("testInfo"))

	// coverage for contextLogger
	getLogger(context.Background()).Warn("testWarn")
	getLogger(context.Background()).Error("testError")
}

// coverage for deprecated SetLoggerFunc
func TestSetLoggerFunc(t *testing.T) {
	SetLoggerFunc(func(ctx context.Context) Logger {
		return getTestLogger(ctx)
	})
	defer SetLoggerFunc(func(ctx context.Context) Logger { return &contextLogger{ctx: ctx, logger: slog.Default()} })
	logger := &testLogger{}
	ctx := context.WithValue(context.Background(), testlogkey, logger)
	getLogger(ctx).Info("test")
	g := NewWithT(t)
	g.Expect(getLogger(ctx)).To(Equal(logger))
	g.Expect(logger.entries).To(HaveLen(1))
	g.Expect(logger.entries[0]).To(Equal("test"))

	g.Expect(func() { getLogger(ctx).Info("") }).ToNot(Panic())

	// getLogger returns nil
	nilCtx := context.WithValue(context.Background(), testlogkey, nil)
	g.Expect(func() { getLogger(nilCtx).Info("test") }).ToNot(Panic())

	//coverage on testlogger
	logger.Warn("test")
	logger.Error("test")
}

type testLogger struct {
	entries []string
}

var testlogkey = struct{}{}

func (t *testLogger) Info(s string) {
	t.entries = append(t.entries, s)
}

func (t *testLogger) Warn(s string) {
}

func (t *testLogger) Error(s string) {
}

func getTestLogger(ctx context.Context) Logger {
	if l, ok := ctx.Value(testlogkey).(*testLogger); ok {
		return l
	}
	return &contextLogger{ctx: ctx, logger: slog.Default()}
}
