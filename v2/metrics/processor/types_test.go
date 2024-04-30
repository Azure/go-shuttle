package processor

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
	r := NewRegistry()
	fRegistry := &fakeRegistry{}
	g.Expect(func() { r.Init(prometheus.NewRegistry()) }).ToNot(Panic())
	g.Expect(func() { r.Init(fRegistry) }).ToNot(Panic())
	g.Expect(fRegistry.collectors).To(HaveLen(6))
	Metric.IncMessageReceived("testReceiverName", 10)
}

func TestNewInformerDefault(t *testing.T) {
	i := NewInformer()
	g := NewWithT(t)
	g.Expect(i.registry).To(Equal(Metric))
}

func TestLockRenewalMetrics(t *testing.T) {
	type testcase struct {
		name string
		msg  *azservicebus.ReceivedMessage
	}
	for _, tc := range []testcase{
		{
			name: "no type property",
			msg:  &azservicebus.ReceivedMessage{},
		},
		{
			name: "with type property",
			msg: &azservicebus.ReceivedMessage{
				ApplicationProperties: map[string]interface{}{
					"type": "someType",
				},
			},
		},
	} {
		g := NewWithT(t)
		r := NewRegistry()
		registerer := prometheus.NewRegistry()
		informer := NewInformerFor(r)

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
		r.IncMessageLockRenewedFailure(tc.msg)
		count, err = informer.GetMessageLockRenewedFailureCount()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(1)))

		// count failure only
		r.IncMessageLockRenewedSuccess(tc.msg)
		count, err = informer.GetMessageLockRenewedFailureCount()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(1)))
	}
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
		r := NewRegistry()
		registerer := prometheus.NewRegistry()
		informer := &Informer{registry: r}

		// before init
		count, err := informer.GetHealthCheckSuccessCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))
		count, err = informer.GetHealthCheckFailureCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))

		// after init, count 0
		g.Expect(func() { r.Init(registerer) }).ToNot(Panic())
		count, err = informer.GetHealthCheckSuccessCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))
		count, err = informer.GetHealthCheckFailureCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))

		// success count incremented
		r.IncHealthCheckSuccessCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		count, err = informer.GetHealthCheckSuccessCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(1)))
		count, err = informer.GetHealthCheckFailureCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))

		// success count incremented
		r.IncHealthCheckSuccessCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		count, err = informer.GetHealthCheckSuccessCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(2)))
		count, err = informer.GetHealthCheckFailureCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))

		// failure count incremented
		r.IncHealthCheckFailureCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		count, err = informer.GetHealthCheckSuccessCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(0)))
		count, err = informer.GetHealthCheckFailureCount(tc.namespaceName, tc.entityName, tc.subscriptionName)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(float64(1)))
	}
}
