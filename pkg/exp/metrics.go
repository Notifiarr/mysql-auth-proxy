package exp

import (
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
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.DelMiss), label, "delete_misses")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Pruned), label, "pruned")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Prunes), label, "prunes")
		ch <- prometheus.MustNewConstMetric(c.counter, prometheus.CounterValue, float64(cache.Pruning.Nanoseconds()), label, "pruning")
	}
}

type Metrics struct {
	Requests     *prometheus.CounterVec
	Queries      *prometheus.CounterVec
	QueryErrors  *prometheus.CounterVec
	QueryMissing *prometheus.CounterVec
	QueryTime    *prometheus.HistogramVec
	ReqTime      *prometheus.HistogramVec
	Cache        *prometheus.CounterVec
}

func GetMetrics(collector *CacheCollector) *Metrics {
	collector.counter = prometheus.NewDesc("cache_counters", "All cache counters", []string{"cache", "counter"}, nil)
	prometheus.MustRegister(collector)

	return &Metrics{
		Requests: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "auth_user_requests_total",
			Help: "The total number of user auth requests processed",
		}, []string{"cache"}),
		Queries: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "db_user_queries_total",
			Help: "The total number of user DB queries performed",
		}, []string{"cache"}),
		QueryErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "db_user_query_errors_total",
			Help: "The total number of user DB query errors",
		}, []string{"cache"}),
		QueryMissing: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "db_user_query_missing_total",
			Help: "The total number of user DB queries with missing user",
		}, []string{"cache"}),
		QueryTime: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "db_user_query_time_seconds",
			Help:    "The duration of user database queries",
			Buckets: []float64{0.01, 0.05, .1, .2, .5, 1, 5},
		}, []string{"cache"}),
		ReqTime: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "auth_user_request_time_seconds",
			Help:    "The duration of user auth requests",
			Buckets: []float64{0.01, 0.05, .1, .2, .5, 1, 5},
		}, []string{"cache"}),
	}
}
