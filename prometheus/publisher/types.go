package publisher

import (
	servicebus "github.com/Azure/azure-service-bus-go"
	prom "github.com/prometheus/client_golang/prometheus"
)

const (
	subsystem            = "goshuttle_publisher"
	messageTypeLabel     = "messageType"
	errorTypeLabel       = "errorType"
	recoverySuccessLabel = "success"
)

type Recorder interface {
	Init(registerer prom.Registerer)
	IncConnectionRecoverySuccess(err error)
	IncConnectionRecoveryFailure(err error)
	IncMessagePublishedSuccess(msg *servicebus.Message)
	IncMessagePublishedFailure(msg *servicebus.Message)
}

var Metrics Recorder = newRegistry()

func newRegistry() *Registry {
	return &Registry{
		MessagePublishedCount: prom.NewCounterVec(prom.CounterOpts{
			Name:      "message_published_total",
			Help:      "total number of messages published by this endpoint",
			Subsystem: subsystem,
		}, []string{messageTypeLabel}),
		ConnectionRecovery: prom.NewCounterVec(prom.CounterOpts{
			Name:      "connection_recovery_total",
			Help:      "total number of connection recovery event",
			Subsystem: subsystem,
		}, []string{errorTypeLabel, recoverySuccessLabel}),
	}
}

func labelsFor(msg *servicebus.Message) prom.Labels {
	typeName := msg.UserProperties["type"].(string)
	return map[string]string{
		messageTypeLabel: typeName,
	}
}

func (r *Registry) Init(reg prom.Registerer) {
	reg.MustRegister(
		r.MessagePublishedCount,
		r.ConnectionRecovery)
}

type Registry struct {
	MessagePublishedCount *prom.CounterVec
	ConnectionRecovery    *prom.CounterVec
}

func (r *Registry) IncMessagePublishedSuccess(msg *servicebus.Message) {
	r.MessagePublishedCount.With(labelsFor(msg)).Inc()
}

func (r *Registry) IncMessagePublishedFailure(msg *servicebus.Message) {
	r.MessagePublishedCount.With(labelsFor(msg)).Inc()
}

func (r *Registry) IncConnectionRecoverySuccess(err error) {
	r.ConnectionRecovery.WithLabelValues("", "true").Inc()
}

func (r *Registry) IncConnectionRecoveryFailure(err error) {
	r.ConnectionRecovery.WithLabelValues("", "false").Inc()
}
