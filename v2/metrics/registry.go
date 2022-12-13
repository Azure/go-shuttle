package metrics

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	prom "github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const (
	subsystem          = "goshuttle_handler"
	messageTypeLabel   = "messageType"
	deliveryCountLabel = "deliveryCount"
	successLabel       = "success"
)

var (
	metricsRegistry          = newRegistry()
	Processor       Recorder = metricsRegistry
)

func newRegistry() *Registry {
	return &Registry{
		MessageReceivedCount: prom.NewCounterVec(prom.CounterOpts{
			Name:      "message_received_total",
			Help:      "total number of messages received by the processor",
			Subsystem: subsystem,
		}, []string{}),
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
	}
}

func getMessageTypeLabel(msg *azservicebus.ReceivedMessage) prom.Labels {
	typeName := msg.ApplicationProperties["type"]
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
	MessageReceivedCount        *prom.CounterVec
	MessageHandledCount         *prom.CounterVec
	MessageLockRenewedCount     *prom.CounterVec
	MessageDeadlineReachedCount *prom.CounterVec
	ConcurrentMessageCount      *prom.GaugeVec
	ConnectionRecovery          *prom.CounterVec
}

type Recorder interface {
	Init(registerer prom.Registerer)
	IncMessageDeadlineReachedCount(msg *azservicebus.ReceivedMessage)
	IncMessageLockRenewedFailure(msg *azservicebus.ReceivedMessage)
	IncMessageLockRenewedSuccess(msg *azservicebus.ReceivedMessage)
	DecConcurrentMessageCount(msg *azservicebus.ReceivedMessage)
	IncMessageHandled(msg *azservicebus.ReceivedMessage)
	IncMessageReceived(float64)
	IncConcurrentMessageCount(msg *azservicebus.ReceivedMessage)
}

func (m *Registry) IncMessageLockRenewedSuccess(msg *azservicebus.ReceivedMessage) {
	labels := getMessageTypeLabel(msg)
	labels[successLabel] = "true"
	m.MessageLockRenewedCount.With(labels).Inc()
}

func (m *Registry) IncMessageLockRenewedFailure(msg *azservicebus.ReceivedMessage) {
	labels := getMessageTypeLabel(msg)
	labels[successLabel] = "false"
	m.MessageLockRenewedCount.With(labels).Inc()
}

func (m *Registry) IncMessageHandled(msg *azservicebus.ReceivedMessage) {
	labels := getMessageTypeLabel(msg)
	labels[deliveryCountLabel] = fmt.Sprintf("%d", msg.DeliveryCount)
	m.MessageHandledCount.With(labels).Inc()
}

func (m *Registry) IncConcurrentMessageCount(msg *azservicebus.ReceivedMessage) {
	m.ConcurrentMessageCount.With(getMessageTypeLabel(msg)).Inc()
}

func (m *Registry) DecConcurrentMessageCount(msg *azservicebus.ReceivedMessage) {
	m.ConcurrentMessageCount.With(getMessageTypeLabel(msg)).Dec()
}

func (m *Registry) IncMessageDeadlineReachedCount(msg *azservicebus.ReceivedMessage) {
	labels := getMessageTypeLabel(msg)
	m.MessageDeadlineReachedCount.With(labels).Inc()
}

func (m *Registry) IncMessageReceived(count float64) {
	m.MessageReceivedCount.With(map[string]string{}).Add(count)
}

type Informer struct {
	registry *Registry
}

func NewInformer() *Informer {
	return &Informer{registry: metricsRegistry}
}

func (i *Informer) GetMessageLockRenewedFailureCount() (float64, error) {
	var total float64
	collect(i.registry.MessageLockRenewedCount, func(m dto.Metric) {
		if !hasLabel(m, successLabel, "false") {
			return
		}
		total += m.GetCounter().GetValue()
	})
	return total, nil
}

func hasLabel(m dto.Metric, key string, value string) bool {
	for _, pair := range m.Label {
		if pair == nil {
			continue
		}
		if pair.GetName() == key && pair.GetValue() == value {
			return true
		}
	}
	return false
}

// collect calls the function for each metric associated with the Collector
func collect(col prom.Collector, do func(dto.Metric)) {
	c := make(chan prom.Metric)
	go func(c chan prom.Metric) {
		col.Collect(c)
		close(c)
	}(c)
	for x := range c { // eg range across distinct label vector values
		m := dto.Metric{}
		_ = x.Write(&m)
		do(m)
	}
}
