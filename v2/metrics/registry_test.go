package metrics

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

func TestRegister(t *testing.T) {
	g := NewWithT(t)
	reg := &fakeRegistry{}
	g.Expect(func() { Register(reg) }).ToNot(Panic())
	g.Expect(reg.collectors).To(HaveLen(6))
}
