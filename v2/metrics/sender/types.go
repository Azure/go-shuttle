package sender

import (
	"github.com/Azure/go-shuttle/v2/metrics/common"
	prom "github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const (
	subsystem      = "goshuttle_handler"
	successLabel   = "success"
	namespaceLabel = "namespace"
	entityLabel    = "entity"
)

var (
	metricsRegistry = newRegistry()
	// Metric exposes a Recorder interface to manipulate the Processor metrics.
	Metric Recorder = metricsRegistry
)

func newRegistry() *Registry {
	return &Registry{
		MessageSentCount: prom.NewCounterVec(prom.CounterOpts{
			Name:      "message_sent_total",
			Help:      "total number of messages sent by the sender",
			Subsystem: subsystem,
		}, []string{successLabel}),
		ConsecutiveConnectionCount: prom.NewGaugeVec(prom.GaugeOpts{
			Name:      "sender_consecutive_connection_count",
			Help:      "number of consecutive connection successes or failures",
			Subsystem: subsystem,
		}, []string{namespaceLabel, entityLabel, successLabel}),
	}
}

func (m *Registry) Init(reg prom.Registerer) {
	reg.MustRegister(
		m.MessageSentCount,
		m.ConsecutiveConnectionCount,
	)
}

type Registry struct {
	MessageSentCount           *prom.CounterVec
	ConsecutiveConnectionCount *prom.GaugeVec
}

// Recorder allows to initialize the metric registry and increase/decrease the registered metrics at runtime.
type Recorder interface {
	Init(registerer prom.Registerer)
	IncSendMessageSuccessCount()
	IncSendMessageFailureCount()
	IncConsecutiveConnectionSuccessCount(namespace, entity string)
	IncConsecutiveConnectionFailureCount(namespace, entity string)
}

// IncSendMessageSuccessCount increases the MessageSentCount metric with success == true
func (m *Registry) IncSendMessageSuccessCount() {
	m.MessageSentCount.With(
		prom.Labels{
			successLabel: "true",
		}).Inc()
}

// IncSendMessageFailureCount increases the MessageSentCount metric with success == false
func (m *Registry) IncSendMessageFailureCount() {
	m.MessageSentCount.With(
		prom.Labels{
			successLabel: "false",
		}).Inc()
}

// IncConsecutiveConnectionSuccessCount increases the connection success gauge and resets the failure gauge
func (m *Registry) IncConsecutiveConnectionSuccessCount(namespace, entity string) {
	labels := map[string]string{
		namespaceLabel: namespace,
		entityLabel:    entity,
		successLabel:   "true",
	}
	m.ConsecutiveConnectionCount.With(labels).Inc()
	// reset the failure count
	labels[successLabel] = "false"
	m.ConsecutiveConnectionCount.With(labels).Set(0)
}

// IncConsecutiveConnectionFailureCount increases the connection failure gauge and resets the success gauge
func (m *Registry) IncConsecutiveConnectionFailureCount(namespace, entity string) {
	labels := map[string]string{
		namespaceLabel: namespace,
		entityLabel:    entity,
		successLabel:   "false",
	}
	m.ConsecutiveConnectionCount.With(labels).Inc()
	// reset the success count
	labels[successLabel] = "true"
	m.ConsecutiveConnectionCount.With(labels).Set(0)
}

// Informer allows to inspect metrics value stored in the registry at runtime
type Informer struct {
	registry *Registry
}

// NewInformer creates an Informer for the current registry
func NewInformer() *Informer {
	return &Informer{registry: metricsRegistry}
}

// GetSendMessageFailureCount returns the total number of messages sent by the sender with success == false
func (i *Informer) GetSendMessageFailureCount() (float64, error) {
	var total float64
	common.Collect(i.registry.MessageSentCount, func(m *dto.Metric) {
		if !common.HasLabel(m, successLabel, "false") {
			return
		}
		total += m.GetCounter().GetValue()
	})
	return total, nil
}

// GetConsecutiveConnectionSuccessCount retrieves the current value of the ConsecutiveConnectionSuccessCount metric
func (i *Informer) GetConsecutiveConnectionSuccessCount(namespace, entity string) (float64, error) {
	var total float64
	common.Collect(i.registry.ConsecutiveConnectionCount, func(m *dto.Metric) {
		labels := map[string]string{
			namespaceLabel: namespace,
			entityLabel:    entity,
			successLabel:   "true",
		}
		if !common.HasLabels(m, labels) {
			return
		}
		total += m.GetGauge().GetValue()
	})
	return total, nil
}

// GetConsecutiveConnectionFailureCount retrieves the current value of the ConsecutiveConnectionFailureCount metric
func (i *Informer) GetConsecutiveConnectionFailureCount(namespace, entity string) (float64, error) {
	var total float64
	common.Collect(i.registry.ConsecutiveConnectionCount, func(m *dto.Metric) {
		labels := map[string]string{
			namespaceLabel: namespace,
			entityLabel:    entity,
			successLabel:   "false",
		}
		if !common.HasLabels(m, labels) {
			return
		}
		total += m.GetGauge().GetValue()
	})
	return total, nil
}
