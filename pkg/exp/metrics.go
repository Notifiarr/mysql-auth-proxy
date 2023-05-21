package exp

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golift.io/cache"
)

type CacheList map[string]func() *cache.Stats

type CacheCollector struct {
	Stats   CacheList
	counter *prometheus.Desc
}

func (c *CacheCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.counter
}

func (c *CacheCollector) Collect(ch chan<- prometheus.Metric) {
	for label, stats := range c.Stats {
		cache := stats()
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Size), label, "size")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Gets), label, "gets")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Hits), label, "hits")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Misses), label, "misses")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Saves), label, "saves")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Updates), label, "updates")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Deletes), label, "deletes")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.DelMiss), label, "delmiss")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Pruned), label, "pruned")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Prunes), label, "prunes")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Pruning.Nanoseconds()), label, "pruning")
	}
}

type Metrics struct {
	QueryErrors  *prometheus.CounterVec
	QueryMissing *prometheus.CounterVec
	QueryTime    *prometheus.HistogramVec
	ReqTime      *prometheus.HistogramVec
	Cache        *prometheus.CounterVec
	Uptime       prometheus.CounterFunc
}

func GetMetrics(collector *CacheCollector) *Metrics {
	start := time.Now()
	collector.counter = prometheus.NewDesc("authproxy_cache_counters", "All cache counters", []string{"cache", "counter"}, nil)
	prometheus.MustRegister(collector)

	return &Metrics{
		QueryErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "authproxy_db_query_errors_total",
			Help: "The total number of DB query errors",
		}, []string{"cache"}),
		QueryMissing: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "authproxy_db_query_missing_total",
			Help: "The total number of DB queries with missing user",
		}, []string{"cache"}),
		QueryTime: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "authproxy_db_query_time_seconds",
			Help:    "The duration of database queries",
			Buckets: []float64{0.001, 0.005, 0.025, .1, .5, 1, 3},
		}, []string{"cache"}),
		ReqTime: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "authproxy_request_time_seconds",
			Help:    "The duration of auth requests",
			Buckets: []float64{0.001, 0.005, 0.025, .1, .5, 1, 3},
		}, []string{"cache"}),
		Uptime: promauto.NewCounterFunc(prometheus.CounterOpts{
			Name: "authproxy_uptime",
			Help: "Seconds the auth proxy has been running",
		}, func() float64 { return time.Since(start).Seconds() }),
	}
}
