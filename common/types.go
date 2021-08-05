package common

import (
	servicebus "github.com/Azure/azure-service-bus-go"
	"time"
)

var _ Listener = &ListenerSettings{}

type Listener interface {
	MaxDeliveryCount() int32
	LockRenewalInterval() *time.Duration
	LockDuration() time.Duration
	PrefetchCount() *uint32
	MaxConcurrency() *int
	Namespace() *servicebus.Namespace
	SetMaxDeliveryCount(maxDeliveryCount int32)
	SetLockRenewalInterval(lockRenewalInterval *time.Duration)
	SetLockDuration(lockDuration time.Duration)
	SetPrefetchCount(prefectCount *uint32)
	SetMaxConcurrency(maxConcurrency *int)
	SetNamespace(namespace *servicebus.Namespace)
}

// ListenerSettings is a struct to contain service bus entities relevant to subscribing to a publisher queue
type ListenerSettings struct {
	maxDeliveryCount    int32
	lockRenewalInterval *time.Duration
	lockDuration        time.Duration
	prefetchCount       *uint32
	maxConcurrency      *int
	namespace           *servicebus.Namespace
}

func (l *ListenerSettings) MaxDeliveryCount() int32 {
	return l.maxDeliveryCount
}

func (l *ListenerSettings) LockRenewalInterval() *time.Duration {
	return l.lockRenewalInterval
}

func (l *ListenerSettings) LockDuration() time.Duration {
	return l.lockDuration
}

func (l *ListenerSettings) PrefetchCount() *uint32 {
	return l.prefetchCount
}

func (l *ListenerSettings) MaxConcurrency() *int {
	return l.maxConcurrency
}

func (l *ListenerSettings) Namespace() *servicebus.Namespace {
	return l.namespace
}

func (l *ListenerSettings) SetMaxDeliveryCount(maxDeliveryCount int32) {
	l.maxDeliveryCount = maxDeliveryCount
}

func (l *ListenerSettings) SetLockRenewalInterval(lockRenewalInterval *time.Duration) {
	l.lockRenewalInterval = lockRenewalInterval
}

func (l *ListenerSettings) SetLockDuration(lockDuration time.Duration) {
	l.lockDuration = lockDuration
}

func (l *ListenerSettings) SetPrefetchCount(prefectCount *uint32) {
	l.prefetchCount = prefectCount
}

func (l *ListenerSettings) SetMaxConcurrency(maxConcurrency *int) {
	l.maxConcurrency = maxConcurrency
}

func (l *ListenerSettings) SetNamespace(namespace *servicebus.Namespace) {
	l.namespace = namespace
}

var _ Publisher = &PublisherSettings{}

type Publisher interface {
	Namespace() *servicebus.Namespace
	Headers() map[string]string
	SetNamespace(namespace *servicebus.Namespace)
	AppendHeader(k, v string)
}

// PublisherSettings is a struct to contain service bus entities relevant to publishing to a queue
type PublisherSettings struct {
	namespace *servicebus.Namespace
	headers   map[string]string
}

func (p *PublisherSettings) Namespace() *servicebus.Namespace {
	return p.namespace
}

func (p *PublisherSettings) Headers() map[string]string {
	return p.headers
}

func (p *PublisherSettings) SetNamespace(namespace *servicebus.Namespace) {
	p.namespace = namespace
}

func (p *PublisherSettings) AppendHeader(key, value string) {
	if p.headers == nil {
		p.headers = make(map[string]string)
	}
	p.headers[key] = value
}
