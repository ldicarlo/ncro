package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// Narinfo requests served from the route cache.
	NarinfoCacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ncro_narinfo_cache_hits_total",
		Help: "Narinfo requests served from route cache.",
	})

	// Narinfo requests that required an upstream race.
	NarinfoCacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ncro_narinfo_cache_misses_total",
		Help: "Narinfo requests requiring upstream race.",
	})

	// Narinfo requests by HTTP status code.
	NarinfoRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ncro_narinfo_requests_total",
		Help: "Narinfo requests by status.",
	}, []string{"status"})

	// NAR streaming requests.
	NARRequests = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "ncro_nar_requests_total",
		Help: "NAR streaming requests.",
	})

	// Times each upstream won the narinfo race.
	UpstreamRaceWins = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ncro_upstream_race_wins_total",
		Help: "Times each upstream won the narinfo race.",
	}, []string{"upstream"})

	// Current number of route entries in SQLite.
	RouteEntries = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ncro_route_entries",
		Help: "Current number of route entries in SQLite.",
	})

	// Upstream narinfo race latency in seconds.
	UpstreamLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ncro_upstream_latency_seconds",
		Help:    "Upstream narinfo race latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"upstream"})
)

// Registers all metrics with reg.
func Register(reg prometheus.Registerer) {
	reg.MustRegister(
		NarinfoCacheHits,
		NarinfoCacheMisses,
		NarinfoRequests,
		NARRequests,
		UpstreamRaceWins,
		RouteEntries,
		UpstreamLatency,
	)
}
