package baseinterfaces

import (
	servicebus "github.com/Azure/azure-service-bus-go"
	"time"
)

var _ BaseListener = &ListenerSettings{}

type BaseListener interface {
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

func (l *ListenerSettings) SetMaxDeliveryCount(maxDeliveryCount int32)  {
	l.maxDeliveryCount = maxDeliveryCount
}

func (l *ListenerSettings) SetLockRenewalInterval(lockRenewalInterval *time.Duration)  {
	l.lockRenewalInterval = lockRenewalInterval
}

func (l *ListenerSettings) SetLockDuration(lockDuration time.Duration)  {
	l.lockDuration = lockDuration
}

func (l *ListenerSettings) SetPrefetchCount(prefectCount *uint32)  {
	l.prefetchCount = prefectCount
}

func (l *ListenerSettings) SetMaxConcurrency(maxConcurrency *int)  {
	l.maxConcurrency = maxConcurrency
}

func (l *ListenerSettings) SetNamespace(namespace *servicebus.Namespace) {
	l.namespace = namespace
}