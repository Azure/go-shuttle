package publisher

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
)

func TestRegistry_Init(t *testing.T) {
	g := NewWithT(t)
	r := newRegistry()
	registerer := prometheus.NewRegistry()
	g.Expect(func() { r.Init(registerer) }).ToNot(Panic())
}

func TestInformer_GetMetric(t *testing.T) {
	g := NewWithT(t)
	r := newRegistry()
	registerer := prometheus.NewRegistry()
	informer := &Informer{registry: r}

	// before init
	count, err := informer.GetConnectionRecoveryFailureCount()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(float64(0)))

	// after init, count 0
	g.Expect(func() { r.Init(registerer) }).ToNot(Panic())
	count, err = informer.GetConnectionRecoveryFailureCount()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(float64(0)))

	// count incremented
	r.IncConnectionRecoveryFailure(fmt.Errorf("some error"))
	count, err = informer.GetConnectionRecoveryFailureCount()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(float64(1)))
}
