// Package route answers "how far is it really?" — road distance and travel
// time between two points. It asks a public OSRM instance when one answers
// within a few seconds and falls back to a calibrated straight-line model
// (haversine × road factor at town-bike speed) when it does not, so the
// dispatch board never stalls on a flaky upstream. Answers are cached per
// rounded pair — the same shop→customer pair repeats across riders' boards.
package route

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/webdevshayan/city-market/server/match"
)

// Calibration constants for the fallback model and the ETA math. The road
// factor folds in real street detours versus the great-circle line; the
// speed is a moped on Johi's town roads, not a highway average.
const (
	RoadFactor   = 1.32               // straight line → rough road distance
	TownSpeedMPS = 24.0 * 1000 / 3600 // 24 km/h average door-to-door
	PickupDwell  = 3 * 60             // seconds the handover at the shop takes
	DropDwell    = 60                 // seconds the handover at the door takes
	osrmTimeout  = 3 * time.Second
	cacheTTL     = 10 * time.Minute
)

// Estimate is one answer: road distance in metres and travel time in
// seconds, handover dwells excluded (callers add those per leg).
type Estimate struct {
	Meters   float64
	Seconds  float64
	FromOSRM bool
}

// Estimator is the surface the rider handlers use.
type Estimator interface {
	Estimate(from, to match.Point) (Estimate, error)
}

// OSRM is the default estimator: live OSRM answers with a bounded cache,
// fallback model underneath.
type OSRM struct {
	base   string
	client *http.Client
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	est Estimate
	at  time.Time
}

// New returns the estimator pointed at the public OSRM demo server.
func New() *OSRM {
	return NewWith("https://router.project-osrm.org", &http.Client{Timeout: osrmTimeout})
}

// NewWith lets tests point the estimator at a stub and control time.
func NewWith(base string, client *http.Client) *OSRM {
	return &OSRM{
		base:   strings.TrimRight(base, "/"),
		client: client,
		now:    time.Now,
		cache:  map[string]cacheEntry{},
	}
}

// Estimate resolves the road distance and time between from and to.
func (e *OSRM) Estimate(from, to match.Point) (Estimate, error) {
	key := cacheKey(from, to)
	e.mu.Lock()
	if hit, ok := e.cache[key]; ok && e.now().Sub(hit.at) < cacheTTL {
		e.mu.Unlock()
		return hit.est, nil
	}
	e.mu.Unlock()

	est, err := e.askOSRM(from, to)
	if err != nil {
		est = fallback(from, to)
	}

	e.mu.Lock()
	if len(e.cache) >= 4096 {
		e.cache = map[string]cacheEntry{} // bounded: a sweep at 4k pairs
	}
	e.cache[key] = cacheEntry{est: est, at: e.now()}
	e.mu.Unlock()
	return est, nil
}

func (e *OSRM) askOSRM(from, to match.Point) (Estimate, error) {
	url := fmt.Sprintf("%s/route/v1/driving/%.6f,%.6f;%.6f,%.6f?overview=false",
		e.base, from.Lng, from.Lat, to.Lng, to.Lat)
	resp, err := e.client.Get(url)
	if err != nil {
		return Estimate{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Estimate{}, fmt.Errorf("route: osrm status %d", resp.StatusCode)
	}
	var parsed struct {
		Routes []struct {
			Distance float64 `json:"distance"`
			Duration float64 `json:"duration"`
		} `json:"routes"`
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Estimate{}, err
	}
	if parsed.Code != "Ok" || len(parsed.Routes) == 0 {
		return Estimate{}, fmt.Errorf("route: osrm code %s", parsed.Code)
	}
	r := parsed.Routes[0]
	if r.Distance <= 0 || r.Duration <= 0 {
		return Estimate{}, fmt.Errorf("route: osrm degenerate answer")
	}
	return Estimate{Meters: r.Distance, Seconds: r.Duration, FromOSRM: true}, nil
}

// FallbackEstimate is the calibrated straight-line model — always
// available, honest about being a model (FromOSRM=false so UIs can show
// "estimate"). Handlers reach for it when even the estimator is wired
// away.
func FallbackEstimate(from, to match.Point) Estimate {
	meters := match.HaversineMeters(from, to) * RoadFactor
	return Estimate{Meters: meters, Seconds: meters / TownSpeedMPS}
}

func fallback(from, to match.Point) Estimate { return FallbackEstimate(from, to) }

// cacheKey rounds both points to ~11 m grid cells so near-identical
// questions share one answer.
func cacheKey(from, to match.Point) string {
	round := func(v float64) float64 { return math.Round(v*10_000) / 10_000 }
	return fmt.Sprintf("%.4f,%.4f->%.4f,%.4f", round(from.Lat), round(from.Lng), round(to.Lat), round(to.Lng))
}
