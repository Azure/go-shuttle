package prometheus

import (
	"github.com/Azure/go-shuttle/prometheus/listener"
	"github.com/Azure/go-shuttle/prometheus/publisher"
	"github.com/prometheus/client_golang/prometheus"
)

func Register(registerer prometheus.Registerer) {
	listener.Metrics.Init(registerer)
	publisher.Metrics.Init(registerer)
}
