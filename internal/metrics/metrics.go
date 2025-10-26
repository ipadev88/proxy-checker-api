package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Collector struct {
	// Proxy checking metrics
	checksTotal    *prometheus.CounterVec
	checksSuccess  prometheus.Counter
	checksFailure  prometheus.Counter
	checkDuration  prometheus.Histogram
	
	// Proxy stats
	aliveProxies   prometheus.Gauge
	deadProxies    prometheus.Gauge
	
	// Aggregation metrics
	proxiesScraped *prometheus.CounterVec
	
	// Zmap metrics
	zmapScansTotal      *prometheus.CounterVec
	zmapCandidatesFound *prometheus.GaugeVec
	zmapScanDuration    prometheus.Histogram
	
	// API metrics
	apiRequests    *prometheus.CounterVec
	apiDuration    *prometheus.HistogramVec
}

func NewCollector(namespace string) *Collector {
	c := &Collector{
		checksTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "checks_total",
				Help:      "Total number of proxy checks",
			},
			[]string{"result"},
		),
		checksSuccess: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "checks_success_total",
				Help:      "Total number of successful proxy checks",
			},
		),
		checksFailure: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "checks_failure_total",
				Help:      "Total number of failed proxy checks",
			},
		),
		checkDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "check_duration_seconds",
				Help:      "Proxy check duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
		),
		aliveProxies: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "alive_proxies",
				Help:      "Current number of alive proxies",
			},
		),
		deadProxies: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "dead_proxies",
				Help:      "Current number of dead proxies",
			},
		),
		proxiesScraped: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "proxies_scraped_total",
				Help:      "Total number of proxies scraped from sources",
			},
			[]string{"source"},
		),
		zmapScansTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "zmap_scans_total",
				Help:      "Total number of zmap scans executed",
			},
			[]string{"port", "status"},
		),
		zmapCandidatesFound: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "zmap_candidates_found",
				Help:      "Number of candidate proxies found by zmap",
			},
			[]string{"port"},
		),
		zmapScanDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "zmap_scan_duration_seconds",
				Help:      "Duration of zmap scans in seconds",
				Buckets:   []float64{10, 30, 60, 120, 300, 600, 1800, 3600, 7200},
			},
		),
		apiRequests: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "api_requests_total",
				Help:      "Total number of API requests",
			},
			[]string{"method", "endpoint", "status"},
		),
		apiDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "api_request_duration_seconds",
				Help:      "API request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "endpoint"},
		),
	}

	return c
}

func (c *Collector) RecordCheckSuccess() {
	c.checksTotal.WithLabelValues("success").Inc()
	c.checksSuccess.Inc()
}

func (c *Collector) RecordCheckFailure() {
	c.checksTotal.WithLabelValues("failure").Inc()
	c.checksFailure.Inc()
}

func (c *Collector) RecordCheckDuration(seconds float64) {
	c.checkDuration.Observe(seconds)
}

func (c *Collector) SetAliveProxies(count int) {
	c.aliveProxies.Set(float64(count))
}

func (c *Collector) SetDeadProxies(count int) {
	c.deadProxies.Set(float64(count))
}

func (c *Collector) RecordProxiesScraped(source string, count int) {
	c.proxiesScraped.WithLabelValues(source).Add(float64(count))
}

func (c *Collector) RecordAPIRequest(method, endpoint, status string) {
	c.apiRequests.WithLabelValues(method, endpoint, status).Inc()
}

func (c *Collector) RecordAPIDuration(method, endpoint string, seconds float64) {
	c.apiDuration.WithLabelValues(method, endpoint).Observe(seconds)
}

// Zmap metrics methods
func (c *Collector) RecordZmapScan(port int, status string) {
	c.zmapScansTotal.WithLabelValues(fmt.Sprintf("%d", port), status).Inc()
}

func (c *Collector) RecordZmapCandidates(port int, count int) {
	c.zmapCandidatesFound.WithLabelValues(fmt.Sprintf("%d", port)).Set(float64(count))
}

func (c *Collector) RecordZmapDuration(seconds float64) {
	c.zmapScanDuration.Observe(seconds)
}

