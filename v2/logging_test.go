package shuttle

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

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
	return nil
}

func TestSetLoggerFunc(t *testing.T) {
	t.Setenv("GOSHUTTLE_LOG", "ALL")
	SetLoggerFunc(func(ctx context.Context) Logger {
		return getTestLogger(ctx)
	})
	defer SetLoggerFunc(func(_ context.Context) Logger { return &printLogger{} })
	logger := &testLogger{}
	ctx := context.WithValue(context.Background(), testlogkey, logger)
	log(ctx, "test")
	g := NewWithT(t)
	g.Expect(getLogger(ctx)).To(Equal(logger))
	g.Expect(logger.entries).To(HaveLen(1))
	g.Expect(logger.entries[0]).To(Equal("test"))

	g.Expect(func() { log(ctx, nil) }).ToNot(Panic())

	// getLogger returns nil
	nilCtx := context.WithValue(context.Background(), testlogkey, nil)
	g.Expect(func() { log(nilCtx, "test") }).ToNot(Panic())

	//coverage on testlogger
	logger.Warn("test")
	logger.Error("test")
}
