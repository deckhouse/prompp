package cppbridge

import (
	"github.com/golang/protobuf/proto"
	dto "github.com/prometheus/client_model/go"
)

func CppMetrics(f func(metric *dto.Metric) bool) {
	iterator := prometheusMetricsIteratorCtor()
	defer func() { prometheusMetricsIteratorDtor(iterator) }()

	metric := dto.Metric{}
	for {
		bytes := prometheusMetricsIteratorSerialize(iterator)
		if len(bytes) == 0 {
			break
		}

		if err := proto.Unmarshal(bytes, &metric); err == nil {
			if !f(&metric) {
				break
			}

			metric.Reset()
		}
	}
}
