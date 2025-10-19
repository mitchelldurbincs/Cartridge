package promhttp

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// HandlerOpts defines configuration for the HTTP handler.
type HandlerOpts struct{}

// Handler returns an HTTP handler exposing the default registry metrics.
func Handler() http.Handler {
	return HandlerFor(prometheus.DefaultGatherer, HandlerOpts{})
}

// HandlerFor exposes metrics from the provided gatherer using the text exposition format.
func HandlerFor(g prometheus.Gatherer, _ HandlerOpts) http.Handler {
	if g == nil {
		g = prometheus.DefaultGatherer
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		families, err := g.Gather()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		for _, family := range families {
			if family == nil {
				continue
			}
			fmt.Fprintf(w, "# HELP %s %s\n", family.Name, family.Help)
			fmt.Fprintf(w, "# TYPE %s %s\n", family.Name, string(family.Type))
			for _, metric := range family.Metrics {
				switch family.Type {
				case prometheus.CounterValue:
					fmt.Fprintf(w, "%s%s %g\n", family.Name, formatLabels(metric.Labels), metric.Value)
				case prometheus.HistogramValue:
					for _, bucket := range metric.Buckets {
						labels := copyLabels(metric.Labels)
						if math.IsInf(bucket.UpperBound, 1) {
							labels["le"] = "+Inf"
						} else {
							labels["le"] = strconv.FormatFloat(bucket.UpperBound, 'g', -1, 64)
						}
						fmt.Fprintf(w, "%s_bucket%s %d\n", family.Name, formatLabels(labels), bucket.Count)
					}
					fmt.Fprintf(w, "%s_sum%s %g\n", family.Name, formatLabels(metric.Labels), metric.Sum)
					fmt.Fprintf(w, "%s_count%s %d\n", family.Name, formatLabels(metric.Labels), metric.Count)
				}
			}
		}
	})
}

func copyLabels(labels map[string]string) map[string]string {
	copied := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		copied[k] = v
	}
	return copied
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("{")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(k)
		b.WriteString("=\"")
		b.WriteString(labels[k])
		b.WriteString("\"")
	}
	b.WriteString("}")
	return b.String()
}
