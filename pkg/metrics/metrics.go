package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	prefix = "kw_vop_"
	labels = []string{"namespace", "name", "error"}

	ReconcileCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: genMetricName("reconcile_count"),
		Help: "Counter on how many times the reconcile loop has occur.",
	}, labels)

	ReconcileDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    genMetricName("reconcile_duration"),
		Help:    "Histogram on how much each reconcile loop lasts.",
		Buckets: []float64{.1, .25, .5, 1, 2.5, 5, 10},
	}, labels)
)

func init() {
	metrics.Registry.MustRegister(ReconcileCount, ReconcileDuration)
}

func genMetricName(n string) string {
	return fmt.Sprintf("%s%s", prefix, n)
}
