package processor

import (
	"fmt"
	"strconv"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	prom "github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const (
	subsystem          = "goshuttle_handler"
	receiverNameLabel  = "receiverName"
	messageTypeLabel   = "messageType"
	deliveryCountLabel = "deliveryCount"
	successLabel       = "success"
)

var (
	metricsRegistry = NewRegistry()
	// Metric exposes a Recorder interface to manipulate the Processor metrics.
	Metric Recorder = metricsRegistry
)

// NewRegistry creates a new Registry with initialized prometheus counter definitions
func NewRegistry() *Registry {
	return &Registry{
		MessageReceivedCount: prom.NewCounterVec(prom.CounterOpts{
			Name:      "message_received_total",
			Help:      "total number of messages received by the processor",
			Subsystem: subsystem,
		}, []string{receiverNameLabel}),
		MessageHandledCount: prom.NewCounterVec(prom.CounterOpts{
			Name:      "message_handled_total",
			Help:      "total number of messages handled by this handler",
			Subsystem: subsystem,
		}, []string{receiverNameLabel, messageTypeLabel, deliveryCountLabel}),
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
		}, []string{receiverNameLabel, messageTypeLabel}),
	}
}

func getMessageTypeLabel(msg *azservicebus.ReceivedMessage) prom.Labels {
	typeName := msg.ApplicationProperties["type"]
	return map[string]string{
		messageTypeLabel: fmt.Sprintf("%s", typeName),
	}
}

// Init registers the counters from the Registry on the prometheus.Registerer
func (m *Registry) Init(reg prom.Registerer) {
	reg.MustRegister(
		m.MessageReceivedCount,
		m.MessageHandledCount,
		m.MessageLockRenewedCount,
		m.MessageDeadlineReachedCount,
		m.ConcurrentMessageCount)
}

// Registry provides the prometheus metrics for the message processor
type Registry struct {
	MessageReceivedCount        *prom.CounterVec
	MessageHandledCount         *prom.CounterVec
	MessageLockRenewedCount     *prom.CounterVec
	MessageDeadlineReachedCount *prom.CounterVec
	ConcurrentMessageCount      *prom.GaugeVec
}

// Recorder allows to initialize the metric registry and increase/decrease the registered metrics at runtime.
type Recorder interface {
	Init(registerer prom.Registerer)
	IncMessageDeadlineReachedCount(msg *azservicebus.ReceivedMessage)
	IncMessageLockRenewedFailure(msg *azservicebus.ReceivedMessage)
	IncMessageLockRenewedSuccess(msg *azservicebus.ReceivedMessage)
	IncMessageHandled(receiverName string, msg *azservicebus.ReceivedMessage)
	IncMessageReceived(receiverName string, count float64)
	IncConcurrentMessageCount(receiverName string, msg *azservicebus.ReceivedMessage)
	DecConcurrentMessageCount(receiverName string, msg *azservicebus.ReceivedMessage)
}

// IncMessageLockRenewedSuccess increase the message lock renewal success counter
func (m *Registry) IncMessageLockRenewedSuccess(msg *azservicebus.ReceivedMessage) {
	labels := getMessageTypeLabel(msg)
	labels[successLabel] = "true"
	m.MessageLockRenewedCount.With(labels).Inc()
}

// IncMessageLockRenewedFailure increase the message lock renewal failure counter
func (m *Registry) IncMessageLockRenewedFailure(msg *azservicebus.ReceivedMessage) {
	labels := getMessageTypeLabel(msg)
	labels[successLabel] = "false"
	m.MessageLockRenewedCount.With(labels).Inc()
}

// IncMessageHandled increase the message Handled
func (m *Registry) IncMessageHandled(receiverName string, msg *azservicebus.ReceivedMessage) {
	labels := getMessageTypeLabel(msg)
	labels[receiverNameLabel] = receiverName
	labels[deliveryCountLabel] = strconv.FormatUint(uint64(msg.DeliveryCount), 10)
	m.MessageHandledCount.With(labels).Inc()
}

// IncConcurrentMessageCount increases the concurrent message counter
func (m *Registry) IncConcurrentMessageCount(receiverName string, msg *azservicebus.ReceivedMessage) {
	labels := getMessageTypeLabel(msg)
	labels[receiverNameLabel] = receiverName
	m.ConcurrentMessageCount.With(labels).Inc()
}

// DecConcurrentMessageCount decreases the concurrent message counter
func (m *Registry) DecConcurrentMessageCount(receiverName string, msg *azservicebus.ReceivedMessage) {
	labels := getMessageTypeLabel(msg)
	labels[receiverNameLabel] = receiverName
	m.ConcurrentMessageCount.With(labels).Dec()
}

// IncMessageDeadlineReachedCount increases the message deadline reached counter
func (m *Registry) IncMessageDeadlineReachedCount(msg *azservicebus.ReceivedMessage) {
	labels := getMessageTypeLabel(msg)
	m.MessageDeadlineReachedCount.With(labels).Inc()
}

// IncMessageReceived increases the message received counter
func (m *Registry) IncMessageReceived(receiverName string, count float64) {
	m.MessageReceivedCount.WithLabelValues(receiverName).Add(count)
}

// Informer allows to inspect metrics value stored in the registry at runtime
type Informer struct {
	registry *Registry
}

// NewInformer creates an Informer for the current registry
func NewInformer() *Informer {
	return NewInformerFor(metricsRegistry)
}

// NewInformerFor creates an Informer for the current registry
func NewInformerFor(r *Registry) *Informer {
	return &Informer{registry: r}
}

// GetMessageLockRenewedFailureCount retrieves the current value of the MessageLockRenewedFailureCount metric
func (i *Informer) GetMessageLockRenewedFailureCount() (float64, error) {
	var total float64
	collect(i.registry.MessageLockRenewedCount, func(m *dto.Metric) {
		if !hasLabel(m, successLabel, "false") {
			return
		}
		total += m.GetCounter().GetValue()
	})
	return total, nil
}

func hasLabel(m *dto.Metric, key string, value string) bool {
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
func collect(col prom.Collector, do func(*dto.Metric)) {
	c := make(chan prom.Metric)
	go func(c chan prom.Metric) {
		col.Collect(c)
		close(c)
	}(c)
	for x := range c { // eg range across distinct label vector values
		m := &dto.Metric{}
		_ = x.Write(m)
		do(m)
	}
}
