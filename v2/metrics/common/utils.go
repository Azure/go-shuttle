package common

import (
	prom "github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func HasLabel(m *dto.Metric, key string, value string) bool {
	for _, pair := range m.Label {
		if pair == nil {
			continue
		}
		if pair.GetName() == key && pair.GetValue() == value {
			return true
		}
	}
	return false
}

func HasLabels(m *dto.Metric, labels map[string]string) bool {
	for key, value := range labels {
		if !HasLabel(m, key, value) {
			return false
		}
	}
	return true
}

// collect calls the function for each metric associated with the Collector
func Collect(col prom.Collector, do func(*dto.Metric)) {
	c := make(chan prom.Metric)
	go func(c chan prom.Metric) {
		col.Collect(c)
		close(c)
	}(c)
	for x := range c { // eg range across distinct label vector values
		m := &dto.Metric{}
		_ = x.Write(m)
		do(m)
	}
}
