package listener

import (
	"testing"

	servicebus "github.com/Azure/azure-service-bus-go"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
)

type fakeRegistry struct {
	collectors []prometheus.Collector
}

func (f *fakeRegistry) Register(c prometheus.Collector) error {
	panic("implement me")
}

func (f *fakeRegistry) MustRegister(c ...prometheus.Collector) {
	f.collectors = append(f.collectors, c...)
}

func (f *fakeRegistry) Unregister(c prometheus.Collector) bool {
	panic("implement me")
}

func TestRegistry_Init(t *testing.T) {
	g := NewWithT(t)
	r := newRegistry()
	fRegistry := &fakeRegistry{}
	g.Expect(func() { r.Init(prometheus.NewRegistry()) }).ToNot(Panic())
	g.Expect(func() { r.Init(fRegistry) }).ToNot(Panic())
	g.Expect(fRegistry.collectors).To(HaveLen(5))
}

func TestInformer_GetMetric(t *testing.T) {
	g := NewWithT(t)
	r := newRegistry()
	registerer := prometheus.NewRegistry()
	informer := &Informer{registry: r}

	// before init
	count, err := informer.GetMessageLockRenewedFailureCount()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(float64(0)))

	// after init, count 0
	g.Expect(func() { r.Init(registerer) }).ToNot(Panic())
	count, err = informer.GetMessageLockRenewedFailureCount()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(float64(0)))

	// count incremented
	msg := &servicebus.Message{}
	msg.UserProperties = map[string]interface{}{
		"type": "someType",
	}
	r.IncMessageLockRenewedFailure(msg)
	count, err = informer.GetMessageLockRenewedFailureCount()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(float64(1)))

	// count failure only
	r.IncMessageLockRenewedSuccess(msg)
	count, err = informer.GetMessageLockRenewedFailureCount()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(float64(1)))
}
