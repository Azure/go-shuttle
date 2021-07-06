package listener

import (
	"fmt"

	prom "github.com/prometheus/client_golang/prometheus"

	servicebus "github.com/Azure/azure-service-bus-go"
)

const (
	subsystem          = "goshuttle_handler"
	messageTypeLabel   = "messageType"
	deliveryCountLabel = "deliveryCount"
	successLabel       = "success"
)

var Metrics Recorder = newRegistry()

func newRegistry() *Registry {
	return &Registry{
		MessageHandledCount: prom.NewCounterVec(prom.CounterOpts{
			Name:      "message_handled_total",
			Help:      "total number of messages handled by this handler",
			Subsystem: subsystem,
		}, []string{messageTypeLabel, deliveryCountLabel}),
		MessageLockRenewedCount: prom.NewCounterVec(prom.CounterOpts{
			Name:      "message_lock_renewed_total",
			Help:      "total number of message lock renewal",
			Subsystem: subsystem,
		}, []string{messageTypeLabel, successLabel}),
		MessageDeadlineReachedCount: prom.NewCounterVec(prom.CounterOpts{
			Name:      "message_deadline_reached_total",
			Help:      "total number of message lock renewal",
			Subsystem: subsystem,
		}, []string{messageTypeLabel}),
		ConcurrentMessageCount: prom.NewGaugeVec(prom.GaugeOpts{
			Name:      "concurrent_message_count",
			Help:      "number of messages being handled concurrently",
			Subsystem: subsystem,
		}, []string{messageTypeLabel}),
		ConnectionRecovery: prom.NewCounterVec(prom.CounterOpts{
			Name:      "connection_recovery_total",
			Help:      "total number of connection recovery event",
			Subsystem: subsystem,
		}, []string{messageTypeLabel, successLabel}),
	}
}

func getMessageTypeLabel(msg *servicebus.Message) prom.Labels {
	typeName := msg.UserProperties["type"]
	return map[string]string{
		messageTypeLabel: fmt.Sprintf("%s", typeName),
	}
}

func (m *Registry) Init(reg prom.Registerer) {
	reg.MustRegister(m.MessageHandledCount,
		m.MessageLockRenewedCount,
		m.MessageDeadlineReachedCount,
		m.ConcurrentMessageCount,
		m.ConnectionRecovery)
}

type Registry struct {
	MessageHandledCount         *prom.CounterVec
	MessageLockRenewedCount     *prom.CounterVec
	MessageDeadlineReachedCount *prom.CounterVec
	ConcurrentMessageCount      *prom.GaugeVec
	ConnectionRecovery          *prom.CounterVec
}

type Recorder interface {
	Init(registerer prom.Registerer)
	IncMessageDeadlineReachedCount(msg *servicebus.Message)
	IncMessageLockRenewedFailure(msg *servicebus.Message)
	IncMessageLockRenewedSuccess(msg *servicebus.Message)
	DecConcurrentMessageCount(msg *servicebus.Message)
	IncMessageHandled(msg *servicebus.Message)
	IncConcurrentMessageCount(msg *servicebus.Message)
}

func (m *Registry) IncMessageLockRenewedSuccess(msg *servicebus.Message) {
	labels := getMessageTypeLabel(msg)
	labels[successLabel] = "true"
	m.MessageLockRenewedCount.With(labels).Inc()
}

func (m *Registry) IncMessageLockRenewedFailure(msg *servicebus.Message) {
	labels := getMessageTypeLabel(msg)
	labels[successLabel] = "false"
	m.MessageLockRenewedCount.With(labels).Inc()
}

func (m *Registry) IncMessageHandled(msg *servicebus.Message) {
	labels := getMessageTypeLabel(msg)
	labels[deliveryCountLabel] = fmt.Sprintf("%d", msg.DeliveryCount)
	m.MessageHandledCount.With(labels).Inc()
}

func (m *Registry) IncConcurrentMessageCount(msg *servicebus.Message) {
	m.ConcurrentMessageCount.With(getMessageTypeLabel(msg)).Inc()
}

func (m *Registry) DecConcurrentMessageCount(msg *servicebus.Message) {
	m.ConcurrentMessageCount.With(getMessageTypeLabel(msg)).Dec()
}

func (m *Registry) IncMessageDeadlineReachedCount(msg *servicebus.Message) {
	labels := getMessageTypeLabel(msg)
	m.MessageDeadlineReachedCount.With(labels).Inc()
}
