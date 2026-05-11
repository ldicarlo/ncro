use std::sync::OnceLock;

use prometheus::{
  Encoder,
  HistogramOpts,
  HistogramVec,
  IntCounter,
  IntCounterVec,
  IntGauge,
  Opts,
  Registry,
  TextEncoder,
};

pub struct Metrics {
  registry:                 Registry,
  pub narinfo_cache_hits:   IntCounter,
  pub narinfo_cache_misses: IntCounter,
  pub narinfo_requests:     IntCounterVec,
  pub nar_requests:         IntCounter,
  pub upstream_race_wins:   IntCounterVec,
  pub route_entries:        IntGauge,
  pub upstream_latency:     HistogramVec,
}

static METRICS: OnceLock<Metrics> = OnceLock::new();

#[expect(
  clippy::expect_used,
  reason = "metric names and labels are static constants validated during \
            startup"
)]
pub fn get() -> &'static Metrics {
  METRICS.get_or_init(|| {
    let registry = Registry::new();
    let narinfo_cache_hits = IntCounter::new(
      "ncro_narinfo_cache_hits_total",
      "Narinfo requests served from route cache.",
    )
    .expect("valid metric");
    let narinfo_cache_misses = IntCounter::new(
      "ncro_narinfo_cache_misses_total",
      "Narinfo requests requiring upstream race.",
    )
    .expect("valid metric");
    let narinfo_requests = IntCounterVec::new(
      Opts::new("ncro_narinfo_requests_total", "Narinfo requests by status."),
      &["status"],
    )
    .expect("valid metric");
    let nar_requests =
      IntCounter::new("ncro_nar_requests_total", "NAR streaming requests.")
        .expect("valid metric");
    let upstream_race_wins = IntCounterVec::new(
      Opts::new(
        "ncro_upstream_race_wins_total",
        "Times each upstream won the narinfo race.",
      ),
      &["upstream"],
    )
    .expect("valid metric");
    let route_entries = IntGauge::new(
      "ncro_route_entries",
      "Current number of route entries in SQLite.",
    )
    .expect("valid metric");
    let upstream_latency = HistogramVec::new(
      HistogramOpts::new(
        "ncro_upstream_latency_seconds",
        "Upstream narinfo race latency.",
      ),
      &["upstream"],
    )
    .expect("valid metric");

    for collector in [
      Box::new(narinfo_cache_hits.clone())
        as Box<dyn prometheus::core::Collector>,
      Box::new(narinfo_cache_misses.clone()),
      Box::new(narinfo_requests.clone()),
      Box::new(nar_requests.clone()),
      Box::new(upstream_race_wins.clone()),
      Box::new(route_entries.clone()),
      Box::new(upstream_latency.clone()),
    ] {
      registry.register(collector).expect("register metric");
    }

    Metrics {
      registry,
      narinfo_cache_hits,
      narinfo_cache_misses,
      narinfo_requests,
      nar_requests,
      upstream_race_wins,
      route_entries,
      upstream_latency,
    }
  })
}

#[must_use]
pub fn gather() -> String {
  let mut buf = Vec::new();
  let encoder = TextEncoder::new();
  if encoder.encode(&get().registry.gather(), &mut buf).is_err() {
    return String::new();
  }
  String::from_utf8_lossy(&buf).into_owned()
}
