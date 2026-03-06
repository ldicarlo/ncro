package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"notashelf.dev/ncro/internal/config"
	"notashelf.dev/ncro/internal/metrics"
	"notashelf.dev/ncro/internal/prober"
	"notashelf.dev/ncro/internal/router"
)

// HTTP handler implementing the Nix binary cache protocol.
type Server struct {
	router    *router.Router
	prober    *prober.Prober
	upstreams []config.UpstreamConfig
	client    *http.Client
}

// Creates a Server.
func New(r *router.Router, p *prober.Prober, upstreams []config.UpstreamConfig) *Server {
	return &Server{
		router:    r,
		prober:    p,
		upstreams: upstreams,
		client:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/nix-cache-info":
		s.handleCacheInfo(w, r)
	case path == "/health":
		s.handleHealth(w, r)
	case path == "/metrics":
		promhttp.Handler().ServeHTTP(w, r)
	case strings.HasSuffix(path, ".narinfo"):
		s.handleNarinfo(w, r)
	case strings.HasPrefix(path, "/nar/"):
		s.handleNAR(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleCacheInfo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintln(w, "StoreDir: /nix/store")
	fmt.Fprintln(w, "WantMassQuery: 1")
	fmt.Fprintln(w, "Priority: 30")
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, `{"status":"ok"}`)
}

func (s *Server) handleNarinfo(w http.ResponseWriter, r *http.Request) {
	hash := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".narinfo")

	result, err := s.router.Resolve(hash, s.upstreamURLs())
	if err != nil {
		slog.Warn("narinfo resolve failed", "hash", hash, "error", err)
		metrics.NarinfoRequests.WithLabelValues("error").Inc()
		switch {
		case errors.Is(err, router.ErrNotFound):
			http.NotFound(w, r)
		default:
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
		}
		return
	}

	slog.Info("narinfo routed", "hash", hash, "upstream", result.URL, "cache_hit", result.CacheHit)
	metrics.NarinfoRequests.WithLabelValues("200").Inc()

	if len(result.NarInfoBytes) > 0 {
		w.Header().Set("Content-Type", "text/x-nix-narinfo")
		w.WriteHeader(http.StatusOK)
		w.Write(result.NarInfoBytes)
		return
	}
	s.proxyRequest(w, r, result.URL+r.URL.Path)
}

func (s *Server) handleNAR(w http.ResponseWriter, r *http.Request) {
	metrics.NARRequests.Inc()
	sorted := s.prober.SortedByLatency()
	if len(sorted) == 0 {
		http.Error(w, "no upstreams available", http.StatusServiceUnavailable)
		return
	}
	for _, h := range sorted {
		if h.Status == prober.StatusDown {
			continue
		}
		targetURL := h.URL + r.URL.Path
		req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
		if err != nil {
			continue
		}
		for _, hdr := range []string{"Accept", "Accept-Encoding", "Range"} {
			if v := r.Header.Get(hdr); v != "" {
				req.Header.Set(hdr, v)
			}
		}
		resp, err := s.client.Do(req)
		if err != nil {
			slog.Warn("NAR upstream failed", "upstream", h.URL, "error", err)
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			continue
		}
		defer resp.Body.Close()
		slog.Debug("proxying NAR", "path", r.URL.Path, "upstream", h.URL)
		s.copyResponse(w, resp)
		return
	}
	http.NotFound(w, r)
}

// Forwards r to targetURL and streams the response zero-copy.
func (s *Server) proxyRequest(w http.ResponseWriter, r *http.Request, targetURL string) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	for _, h := range []string{"Accept", "Accept-Encoding", "Range"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	resp, err := s.client.Do(req)
	if err != nil {
		slog.Error("upstream request failed", "url", targetURL, "error", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	s.copyResponse(w, resp)
}

// Copies response headers and body from resp to w.
func (s *Server) copyResponse(w http.ResponseWriter, resp *http.Response) {
	for _, h := range []string{
		"Content-Type", "Content-Length", "Content-Encoding",
		"X-Nix-Signature", "Cache-Control", "Last-Modified",
	} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Warn("stream interrupted", "error", err)
	}
}

func (s *Server) upstreamURLs() []string {
	urls := make([]string, len(s.upstreams))
	for i, u := range s.upstreams {
		urls[i] = u.URL
	}
	return urls
}
