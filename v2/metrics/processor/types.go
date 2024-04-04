package processor

import (
	"fmt"
	"strconv"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/go-shuttle/v2/metrics/common"
	prom "github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const (
	subsystem          = "goshuttle_handler"
	messageTypeLabel   = "messageType"
	deliveryCountLabel = "deliveryCount"
	successLabel       = "success"
	namespaceLabel     = "namespace"
	entityLabel        = "entity"
	subscriptionLabel  = "subscription"
)

var (
	metricsRegistry = newRegistry()
	// Metric exposes a Recorder interface to manipulate the Processor metrics.
	Metric Recorder = metricsRegistry
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
		ConsecutiveConnectionCount: prom.NewGaugeVec(prom.GaugeOpts{
			Name:      "receiver_consecutive_connection_count",
			Help:      "number of consecutive connection successes or failures",
			Subsystem: subsystem,
		}, []string{namespaceLabel, entityLabel, subscriptionLabel, successLabel}),
	}
}

func getMessageTypeLabel(msg *azservicebus.ReceivedMessage) prom.Labels {
	typeName := msg.ApplicationProperties["type"]
	return map[string]string{
		messageTypeLabel: fmt.Sprintf("%s", typeName),
	}
}

func (m *Registry) Init(reg prom.Registerer) {
	reg.MustRegister(
		m.MessageReceivedCount,
		m.MessageHandledCount,
		m.MessageLockRenewedCount,
		m.MessageDeadlineReachedCount,
		m.ConcurrentMessageCount,
		m.ConsecutiveConnectionCount)
}

type Registry struct {
	MessageReceivedCount        *prom.CounterVec
	MessageHandledCount         *prom.CounterVec
	MessageLockRenewedCount     *prom.CounterVec
	MessageDeadlineReachedCount *prom.CounterVec
	ConcurrentMessageCount      *prom.GaugeVec
	ConsecutiveConnectionCount  *prom.GaugeVec
}

// Recorder allows to initialize the metric registry and increase/decrease the registered metrics at runtime.
type Recorder interface {
	Init(registerer prom.Registerer)
	IncMessageDeadlineReachedCount(msg *azservicebus.ReceivedMessage)
	IncMessageLockRenewedFailure(msg *azservicebus.ReceivedMessage)
	IncMessageLockRenewedSuccess(msg *azservicebus.ReceivedMessage)
	DecConcurrentMessageCount(msg *azservicebus.ReceivedMessage)
	IncMessageHandled(msg *azservicebus.ReceivedMessage)
	IncMessageReceived(float64)
	IncConcurrentMessageCount(msg *azservicebus.ReceivedMessage)
	IncConsecutiveConnectionSuccessCount(namespace, entity, subscription string)
	IncConsecutiveConnectionFailureCount(namespace, entity, subscription string)
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
func (m *Registry) IncMessageHandled(msg *azservicebus.ReceivedMessage) {
	labels := getMessageTypeLabel(msg)
	labels[deliveryCountLabel] = strconv.FormatUint(uint64(msg.DeliveryCount), 10)
	m.MessageHandledCount.With(labels).Inc()
}

// IncConcurrentMessageCount increases the concurrent message counter
func (m *Registry) IncConcurrentMessageCount(msg *azservicebus.ReceivedMessage) {
	m.ConcurrentMessageCount.With(getMessageTypeLabel(msg)).Inc()
}

// DecConcurrentMessageCount decreases the concurrent message counter
func (m *Registry) DecConcurrentMessageCount(msg *azservicebus.ReceivedMessage) {
	m.ConcurrentMessageCount.With(getMessageTypeLabel(msg)).Dec()
}

// IncMessageDeadlineReachedCount increases the message deadline reached counter
func (m *Registry) IncMessageDeadlineReachedCount(msg *azservicebus.ReceivedMessage) {
	labels := getMessageTypeLabel(msg)
	m.MessageDeadlineReachedCount.With(labels).Inc()
}

// IncMessageReceived increases the message received counter
func (m *Registry) IncMessageReceived(count float64) {
	m.MessageReceivedCount.With(map[string]string{}).Add(count)
}

// IncConsecutiveConnectionSuccessCount increases the connection success gauge and resets the failure gauge
func (m *Registry) IncConsecutiveConnectionSuccessCount(namespace, entity, subscription string) {
	labels := map[string]string{
		namespaceLabel:    namespace,
		entityLabel:       entity,
		subscriptionLabel: subscription,
		successLabel:      "true",
	}
	m.ConsecutiveConnectionCount.With(labels).Inc()
	// reset the failure count
	labels[successLabel] = "false"
	m.ConsecutiveConnectionCount.With(labels).Set(0)
}

// IncConsecutiveConnectionFailureCount increases the connection failure gauge and resets the success gauge
func (m *Registry) IncConsecutiveConnectionFailureCount(namespace, entity, subscription string) {
	labels := map[string]string{
		namespaceLabel:    namespace,
		entityLabel:       entity,
		subscriptionLabel: subscription,
		successLabel:      "false",
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

// GetMessageLockRenewedFailureCount retrieves the current value of the MessageLockRenewedFailureCount metric
func (i *Informer) GetMessageLockRenewedFailureCount() (float64, error) {
	var total float64
	common.Collect(i.registry.MessageLockRenewedCount, func(m *dto.Metric) {
		if !common.HasLabel(m, successLabel, "false") {
			return
		}
		total += m.GetCounter().GetValue()
	})
	return total, nil
}

// GetConsecutiveConnectionSuccessCount retrieves the current value of the ConsecutiveConnectionSuccessCount metric
func (i *Informer) GetConsecutiveConnectionSuccessCount(namespace, entity, subscription string) (float64, error) {
	var total float64
	common.Collect(i.registry.ConsecutiveConnectionCount, func(m *dto.Metric) {
		labels := map[string]string{
			namespaceLabel:    namespace,
			entityLabel:       entity,
			subscriptionLabel: subscription,
			successLabel:      "true",
		}
		if !common.HasLabels(m, labels) {
			return
		}
		total += m.GetGauge().GetValue()
	})
	return total, nil
}

// GetConsecutiveConnectionFailureCount retrieves the current value of the ConsecutiveConnectionFailureCount metric
func (i *Informer) GetConsecutiveConnectionFailureCount(namespace, entity, subscription string) (float64, error) {
	var total float64
	common.Collect(i.registry.ConsecutiveConnectionCount, func(m *dto.Metric) {
		labels := map[string]string{
			namespaceLabel:    namespace,
			entityLabel:       entity,
			subscriptionLabel: subscription,
			successLabel:      "false",
		}
		if !common.HasLabels(m, labels) {
			return
		}
		total += m.GetGauge().GetValue()
	})
	return total, nil
}
