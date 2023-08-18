package sender

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
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
	g.Expect(fRegistry.collectors).To(HaveLen(1))
	Metric.IncSendMessageSuccessCount(&azservicebus.Message{}, "testTopicName")
}

func TestMetrics(t *testing.T) {
	type testcase struct {
		name       string
		msg        *azservicebus.Message
		entityName string
	}
	for _, tc := range []testcase{
		{
			name:       "no type property",
			msg:        &azservicebus.Message{},
			entityName: "testTopicName",
		},
		{
			name: "with type property",
			msg: &azservicebus.Message{
				ApplicationProperties: map[string]interface{}{
					"type": "someType",
				},
			},
			entityName: "testTopicName",
		},
	} {
		g := NewWithT(t)
		r := newRegistry()
		registerer := prometheus.NewRegistry()
		informer := &Informer{registry: r}

		// before init
		count, err := informer.GetSendMessageFailureCount()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))

		// after init, count 0
		g.Expect(func() { r.Init(registerer) }).ToNot(Panic())
		count, err = informer.GetSendMessageFailureCount()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))

		// count incremented
		r.IncSendMessageFailureCount(tc.msg, tc.entityName)
		count, err = informer.GetSendMessageFailureCount()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(1)))

		// count failure only
		r.IncSendMessageSuccessCount(tc.msg, tc.entityName)
		count, err = informer.GetSendMessageFailureCount()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(1)))
	}

}
