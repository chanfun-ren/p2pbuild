package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	defaultBuckets = prometheus.DefBuckets
)

func NewHistogramVec(metricName string) *prometheus.HistogramVec {
	// 动态创建并返回 HistogramVec
	return prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    metricName,
			Help:    "Duration of requests",
			Buckets: defaultBuckets,
		},
		[]string{"method"},
	)
}

func RegisterHistogramVec(metricName string) *prometheus.HistogramVec {
	metric := NewHistogramVec(metricName)
	prometheus.MustRegister(metric)
	return metric
}

func RecordRequestDuration(metric *prometheus.HistogramVec, method string, duration float64) {
	metric.WithLabelValues(method).Observe(duration)
}
