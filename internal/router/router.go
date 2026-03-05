package router

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"notashelf.dev/ncro/internal/cache"
	"notashelf.dev/ncro/internal/prober"
)

// Result of a Resolve call.
type Result struct {
	URL       string
	LatencyMs float64
	CacheHit  bool
}

// Resolves store paths to the best upstream via cache lookup or parallel racing.
type Router struct {
	db          *cache.DB
	prober      *prober.Prober
	routeTTL    time.Duration
	raceTimeout time.Duration
	client      *http.Client
}

// Creates a Router.
func New(db *cache.DB, p *prober.Prober, routeTTL, raceTimeout time.Duration) *Router {
	return &Router{
		db:          db,
		prober:      p,
		routeTTL:    routeTTL,
		raceTimeout: raceTimeout,
		client:      &http.Client{Timeout: raceTimeout},
	}
}

// Returns the best upstream for the given store hash.
// Checks the route cache first; on miss races the provided candidates.
func (r *Router) Resolve(storeHash string, candidates []string) (*Result, error) {
	entry, err := r.db.GetRoute(storeHash)
	if err == nil && entry != nil && entry.IsValid() {
		h := r.prober.GetHealth(entry.UpstreamURL)
		if h == nil || h.Status == prober.StatusActive {
			return &Result{
				URL:       entry.UpstreamURL,
				LatencyMs: entry.LatencyEMA,
				CacheHit:  true,
			}, nil
		}
	}
	return r.race(storeHash, candidates)
}

type raceResult struct {
	url       string
	latencyMs float64
}

func (r *Router) race(storeHash string, candidates []string) (*Result, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates for %q", storeHash)
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.raceTimeout)
	defer cancel()

	ch := make(chan raceResult, len(candidates))
	var wg sync.WaitGroup

	for _, u := range candidates {
		wg.Add(1)
		go func(upstream string) {
			defer wg.Done()
			start := time.Now()
			req, _ := http.NewRequestWithContext(ctx, http.MethodHead,
				upstream+"/"+storeHash+".narinfo", nil)
			resp, err := r.client.Do(req)
			if err != nil {
				return
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				return
			}
			ms := float64(time.Since(start).Nanoseconds()) / 1e6
			select {
			case ch <- raceResult{url: upstream, latencyMs: ms}:
			default:
			}
		}(u)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	winner, ok := <-ch
	if !ok {
		return nil, fmt.Errorf("all upstreams failed for %q", storeHash)
	}
	cancel()

	for res := range ch {
		if res.latencyMs < winner.latencyMs {
			winner = res
		}
	}

	health := r.prober.GetHealth(winner.url)
	ema := winner.latencyMs
	if health != nil {
		ema = 0.3*winner.latencyMs + 0.7*health.EMALatency
	}
	r.prober.RecordLatency(winner.url, winner.latencyMs)

	now := time.Now()
	_ = r.db.SetRoute(&cache.RouteEntry{
		StorePath:    storeHash,
		UpstreamURL:  winner.url,
		LatencyMs:    winner.latencyMs,
		LatencyEMA:   ema,
		LastVerified: now,
		QueryCount:   1,
		TTL:          now.Add(r.routeTTL),
	})

	return &Result{URL: winner.url, LatencyMs: winner.latencyMs}, nil
}
