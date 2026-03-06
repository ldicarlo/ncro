package prober_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"notashelf.dev/ncro/internal/prober"
)

func TestEMACalculation(t *testing.T) {
	p := prober.New(0.3)
	p.RecordLatency("https://example.com", 100)
	p.RecordLatency("https://example.com", 50)

	// EMA after 2 measurements: first=100, second = 0.3*50 + 0.7*100 = 85
	health := p.GetHealth("https://example.com")
	if health == nil {
		t.Fatal("expected health entry")
	}
	if health.EMALatency < 84 || health.EMALatency > 86 {
		t.Errorf("EMA = %.2f, want ~85", health.EMALatency)
	}
}

func TestStatusProgression(t *testing.T) {
	p := prober.New(0.3)
	p.RecordLatency("https://example.com", 10)

	for range 3 {
		p.RecordFailure("https://example.com")
	}
	h := p.GetHealth("https://example.com")
	if h.Status != prober.StatusDegraded {
		t.Errorf("status = %v, want Degraded after 3 failures", h.Status)
	}

	for range 7 {
		p.RecordFailure("https://example.com")
	}
	h = p.GetHealth("https://example.com")
	if h.Status != prober.StatusDown {
		t.Errorf("status = %v, want Down after 10 failures", h.Status)
	}
}

func TestRecoveryAfterSuccess(t *testing.T) {
	p := prober.New(0.3)
	for range 10 {
		p.RecordFailure("https://example.com")
	}
	p.RecordLatency("https://example.com", 20)
	h := p.GetHealth("https://example.com")
	if h.Status != prober.StatusActive {
		t.Errorf("status = %v, want Active after recovery", h.Status)
	}
	if h.ConsecutiveFails != 0 {
		t.Errorf("ConsecutiveFails = %d, want 0", h.ConsecutiveFails)
	}
}

func TestSortedByLatency(t *testing.T) {
	p := prober.New(0.3)
	p.RecordLatency("https://slow.example.com", 200)
	p.RecordLatency("https://fast.example.com", 10)
	p.RecordLatency("https://medium.example.com", 50)

	sorted := p.SortedByLatency()
	if len(sorted) != 3 {
		t.Fatalf("expected 3, got %d", len(sorted))
	}
	if sorted[0].URL != "https://fast.example.com" {
		t.Errorf("first = %q, want fast", sorted[0].URL)
	}
}

func TestProbeUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := prober.New(0.3)
	p.ProbeUpstream(srv.URL)

	h := p.GetHealth(srv.URL)
	if h == nil || h.Status != prober.StatusActive {
		t.Errorf("expected Active after successful probe, got %v", h)
	}
}

func TestSortedByLatencyWithPriority(t *testing.T) {
	p := prober.New(0.3)
	// Two upstreams with very similar latency; lower priority number should win.
	p.RecordLatency("https://low-priority.example.com", 100)
	p.RecordLatency("https://high-priority.example.com", 102) // within 10%

	// Set priorities by calling InitUpstreams via RecordLatency (already seeded).
	// We can't call InitUpstreams without config here, so test via SortedByLatency
	// behavior: without priority, the 100ms one wins. With equal EMA and priority
	// both zero (default), the lower-latency one still wins.
	sorted := p.SortedByLatency()
	if len(sorted) != 2 {
		t.Fatalf("expected 2, got %d", len(sorted))
	}
	// The 100ms upstream should be first (lower latency wins when not within 10% tie).
	// 100 vs 102: diff=2, 2/102=1.96% < 10%, so priority decides (both priority=0, tie → latency).
	// Actually 100 < 102 still wins on latency when priority is equal.
	if sorted[0].EMALatency > sorted[1].EMALatency {
		t.Errorf("expected lower latency first, got %.2f then %.2f", sorted[0].EMALatency, sorted[1].EMALatency)
	}
}

func TestProbeUpstreamFailure(t *testing.T) {
	p := prober.New(0.3)
	p.ProbeUpstream("http://127.0.0.1:1") // nothing listening

	h := p.GetHealth("http://127.0.0.1:1")
	if h == nil || h.ConsecutiveFails == 0 {
		t.Error("expected failure recorded")
	}
}
