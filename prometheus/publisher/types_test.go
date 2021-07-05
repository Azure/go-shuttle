package publisher

import (
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
