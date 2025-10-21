package http

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "web_http_request_duration_seconds",
			Help:    "Latency for HTTP requests.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"handler", "code"},
	)
	requestTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "web_http_requests_total",
			Help: "Total number of HTTP requests handled.",
		},
		[]string{"handler", "code"},
	)
)

func init() {
	prometheus.DefaultRegisterer.MustRegister(requestDuration, requestTotal)
}
