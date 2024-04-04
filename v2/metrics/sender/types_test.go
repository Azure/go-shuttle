package sender

import (
	"testing"

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
	g.Expect(fRegistry.collectors).To(HaveLen(2))
	Metric.IncSendMessageSuccessCount()
}

func TestSendMetrics(t *testing.T) {
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
	r.IncSendMessageFailureCount()
	count, err = informer.GetSendMessageFailureCount()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(float64(1)))

	// count failure only
	r.IncSendMessageSuccessCount()
	count, err = informer.GetSendMessageFailureCount()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(float64(1)))
}

func TestConnectionMetrics(t *testing.T) {
	type testcase struct {
		namespaceName    string
		entityName       string
		subscriptionName string
	}
	for _, tc := range []testcase{
		{
			namespaceName:    "namespace",
			entityName:       "entity",
			subscriptionName: "",
		},
		{
			namespaceName:    "namespace",
			entityName:       "entity",
			subscriptionName: "subscription",
		},
	} {
		g := NewWithT(t)
		r := newRegistry()
		registerer := prometheus.NewRegistry()
		informer := &Informer{registry: r}

		// before init
		count, err := informer.GetConsecutiveConnectionSuccessCount(tc.namespaceName, tc.entityName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))
		count, err = informer.GetConsecutiveConnectionFailureCount(tc.namespaceName, tc.entityName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))

		// after init, count 0
		g.Expect(func() { r.Init(registerer) }).ToNot(Panic())
		count, err = informer.GetConsecutiveConnectionSuccessCount(tc.namespaceName, tc.entityName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))
		count, err = informer.GetConsecutiveConnectionFailureCount(tc.namespaceName, tc.entityName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))

		// success count incremented
		r.IncConsecutiveConnectionSuccessCount(tc.namespaceName, tc.entityName)
		count, err = informer.GetConsecutiveConnectionSuccessCount(tc.namespaceName, tc.entityName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(1)))
		count, err = informer.GetConsecutiveConnectionFailureCount(tc.namespaceName, tc.entityName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))

		// success count incremented
		r.IncConsecutiveConnectionSuccessCount(tc.namespaceName, tc.entityName)
		count, err = informer.GetConsecutiveConnectionSuccessCount(tc.namespaceName, tc.entityName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(2)))
		count, err = informer.GetConsecutiveConnectionFailureCount(tc.namespaceName, tc.entityName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))

		// failure count incremented
		r.IncConsecutiveConnectionFailureCount(tc.namespaceName, tc.entityName)
		count, err = informer.GetConsecutiveConnectionSuccessCount(tc.namespaceName, tc.entityName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))
		count, err = informer.GetConsecutiveConnectionFailureCount(tc.namespaceName, tc.entityName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(1)))
	}
}
