package router_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"notashelf.dev/ncro/internal/cache"
	"notashelf.dev/ncro/internal/prober"
	"notashelf.dev/ncro/internal/router"
)

func newTestRouter(t *testing.T, upstreams ...string) (*router.Router, func()) {
	t.Helper()
	f, _ := os.CreateTemp("", "ncro-router-*.db")
	f.Close()
	db, err := cache.Open(f.Name(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	p := prober.New(0.3)
	for _, u := range upstreams {
		p.RecordLatency(u, 10)
	}
	r := router.New(db, p, time.Hour, 5*time.Second)
	return r, func() {
		db.Close()
		os.Remove(f.Name())
	}
}

func TestRouteHit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "StorePath: /nix/store/abc123-hello")
	}))
	defer srv.Close()

	r, cleanup := newTestRouter(t, srv.URL)
	defer cleanup()

	result, err := r.Resolve("abc123", []string{srv.URL})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.URL != srv.URL {
		t.Errorf("url = %q, want %q", result.URL, srv.URL)
	}
	if result.LatencyMs <= 0 {
		t.Error("expected positive latency")
	}
}

func TestRouteRacePicksFastest(t *testing.T) {
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer fast.Close()

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer slow.Close()

	r, cleanup := newTestRouter(t, fast.URL, slow.URL)
	defer cleanup()

	result, err := r.Resolve("somehash", []string{slow.URL, fast.URL})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.URL != fast.URL {
		t.Errorf("expected fast server to win, got %q", result.URL)
	}
}

func TestRouteAllFail(t *testing.T) {
	r, cleanup := newTestRouter(t)
	defer cleanup()

	_, err := r.Resolve("somehash", []string{"http://127.0.0.1:1"})
	if err == nil {
		t.Error("expected error when all upstreams fail")
	}
}

func TestCacheHit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	r, cleanup := newTestRouter(t, srv.URL)
	defer cleanup()

	r.Resolve("abc123", []string{srv.URL})

	result, err := r.Resolve("abc123", []string{srv.URL})
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if !result.CacheHit {
		t.Error("expected cache hit on second resolve")
	}
}
