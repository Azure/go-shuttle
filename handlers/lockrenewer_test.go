package handlers_test

import (
	"context"
	"testing"
	"time"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-shuttle/handlers"
	"github.com/Azure/go-shuttle/prometheus/listener"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

type NoOpHandler struct {
}

func (h *NoOpHandler) Handle(_ context.Context, _ *servicebus.Message) error {
	return nil
}

type testLockRenewer struct {
	RenewCount int
}

func (t *testLockRenewer) RenewLocks(_ context.Context, _ ...*servicebus.Message) error {
	t.RenewCount++
	return nil
}

var _ handlers.LockRenewer = &testLockRenewer{}

func Test_RenewPeriodically(t *testing.T) {
	renewer := &testLockRenewer{}
	interval := 50 * time.Millisecond
	lr := handlers.NewPeekLockRenewer(&interval, renewer, &NoOpHandler{})
	msg := &servicebus.Message{}
	ctx, _ := context.WithTimeout(context.TODO(), 120*time.Millisecond)
	lr.Handle(ctx, msg)
	if !assert.Eventually(t, func() bool { return renewer.RenewCount == 2 }, 130*time.Millisecond, 20*time.Millisecond) {
		t.Errorf("renewed %d times but expected 2", renewer.RenewCount)
	}
}

func Test_RenewLockMetrics(t *testing.T) {
	reg := prom.NewRegistry()
	listener.Metrics.Init(reg)
	renewer := &testLockRenewer{}
	interval := 10 * time.Millisecond
	lr := handlers.NewPeekLockRenewer(&interval, renewer, &NoOpHandler{})
	msg := &servicebus.Message{}
	lr.Handle(context.TODO(), msg)
	if !assert.Eventually(t, func() bool { return renewer.RenewCount == 2 }, 35*time.Millisecond, 5*time.Millisecond) {
		t.Errorf("renewed %d times but expected 2", renewer.RenewCount)
	}
	metrics, _ := reg.Gather()
	assert.Equal(t, 1, len(metrics))
	assert.Equal(t, "goshuttle_handler_message_lock_renewed_total", *metrics[0].Name)
	assert.Equal(t, float64(2), *metrics[0].Metric[0].Counter.Value)
}

func Test_RenewPeriodically_ContextCanceled(t *testing.T) {
	renewer := &testLockRenewer{}
	interval := 50 * time.Millisecond
	lr := handlers.NewPeekLockRenewer(&interval, renewer, &NoOpHandler{})
	msg := &servicebus.Message{}
	ctx, _ := context.WithTimeout(context.TODO(), 45*time.Millisecond)
	lr.Handle(ctx, msg)
	time.Sleep(90 * time.Millisecond)
	assert.True(t, renewer.RenewCount == 0, "should never renew")
}
