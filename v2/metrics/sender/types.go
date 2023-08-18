package sender

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	prom "github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const (
	subsystem        = "goshuttle_handler"
	messageTypeLabel = "messageType"
	successLabel     = "success"
	entityNameLabel  = "entityName"
)

var (
	metricsRegistry = newRegistry()
	// Processor exposes a Recorder interface to manipulate the Processor metrics.
	Metric Recorder = metricsRegistry
)

func newRegistry() *Registry {
	return &Registry{
		MessageSentCount: prom.NewCounterVec(prom.CounterOpts{
			Name:      "message_sent_total",
			Help:      "total number of messages sent by the sender",
			Subsystem: subsystem,
		}, []string{messageTypeLabel, successLabel, entityNameLabel}),
	}
}

func getMessageTypeLabel(msg *azservicebus.Message) prom.Labels {
	typeName := msg.ApplicationProperties["type"]
	return map[string]string{
		messageTypeLabel: fmt.Sprintf("%s", typeName),
	}
}

func (m *Registry) Init(reg prom.Registerer) {
	reg.MustRegister(
		m.MessageSentCount,
	)
}

type Registry struct {
	MessageSentCount *prom.CounterVec
}

// Recorder allows to initialize the metric registry and increase/decrease the registered metrics at runtime.
type Recorder interface {
	Init(registerer prom.Registerer)
	IncSendMessageSuccessCount(msg *azservicebus.Message, entityName string)
	IncSendMessageFailureCount(msg *azservicebus.Message, entityName string)
}

// IncSendMessageSuccessCount increases the MessageSentCount metric with success == true
func (m *Registry) IncSendMessageSuccessCount(msg *azservicebus.Message, entityName string) {
	labels := getMessageTypeLabel(msg)
	labels[successLabel] = "true"
	labels[entityNameLabel] = entityName
	m.MessageSentCount.With(labels).Inc()
}

// IncSendMessageFailureCount increases the MessageSentCount metric with success == false
func (m *Registry) IncSendMessageFailureCount(msg *azservicebus.Message, entityName string) {
	labels := getMessageTypeLabel(msg)
	labels[successLabel] = "false"
	labels[entityNameLabel] = entityName
	m.MessageSentCount.With(labels).Inc()
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
	collect(i.registry.MessageSentCount, func(m dto.Metric) {
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
