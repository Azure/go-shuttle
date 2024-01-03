package shuttle

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

type testLogger struct {
	entries []interface{}
}

var testlogkey = struct{}{}

func (t *testLogger) Info(args ...interface{}) {
	t.entries = append(t.entries, args...)
}

func (t *testLogger) Debug(args ...interface{}) {
}

func (t *testLogger) Fatal(args ...interface{}) {
}

func (t *testLogger) Panic(args ...interface{}) {
}

func (t *testLogger) Warn(args ...interface{}) {
}

func (t *testLogger) Error(args ...interface{}) {
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
	defer SetLoggerFunc(func(_ context.Context) Logger { return logrus.StandardLogger() })
	logger := &testLogger{}
	ctx := context.WithValue(context.Background(), testlogkey, logger)
	getLogger(ctx).Info("test")
	g := NewWithT(t)
	g.Expect(getLogger(ctx)).To(Equal(logger))
	g.Expect(logger.entries).To(HaveLen(1))
	g.Expect(logger.entries[0]).To(Equal("test"))

	g.Expect(func() { getLogger(ctx).Info(nil) }).ToNot(Panic())

	// getLogger returns nil
	nilCtx := context.WithValue(context.Background(), testlogkey, nil)
	g.Expect(func() { getLogger(nilCtx).Info("test") }).ToNot(Panic())

	//coverage on testlogger
	logger.Warn("test")
	logger.Error("test")
	logger.Fatal("test")
	logger.Panic("test")
	logger.Debug("test")
}
