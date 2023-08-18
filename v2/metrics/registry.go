// Package metrics allows to configure, record and read go-shuttle metrics
package metrics

import (
	"github.com/Azure/go-shuttle/v2/metrics/processor"
	"github.com/Azure/go-shuttle/v2/metrics/sender"
	prom "github.com/prometheus/client_golang/prometheus"
)

// Register registers the go shuttle metrics with the given prometheus registerer.
func Register(reg prom.Registerer) {
	sender.Metric.Init(reg)
	processor.Metric.Init(reg)
}
