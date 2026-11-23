package route

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/webdevshayan/city-market/server/match"
)

func TestFallbackModel(t *testing.T) {
	// One degree of latitude ≈ 111.19 km; the fallback applies the road
	// factor on top of that.
	a := match.Point{Lat: 26.0, Lng: 67.0}
	b := match.Point{Lat: 27.0, Lng: 67.0}
	est := FallbackEstimate(a, b)
	if est.FromOSRM {
		t.Fatal("fallback must never claim OSRM provenance")
	}
	wantM := match.HaversineMeters(a, b) * RoadFactor
	if math.Abs(est.Meters-wantM) > 1 {
		t.Fatalf("meters = %.0f, want %.0f", est.Meters, wantM)
	}
	if est.Meters <= match.HaversineMeters(a, b) {
		t.Fatal("road distance must exceed the straight line")
	}
	if est.Seconds <= 0 {
		t.Fatal("fallback must estimate positive travel time")
	}
}

func TestOSRMAnswersAreUsedAndCached(t *testing.T) {
	calls := 0
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !contains(r.URL.Path, "/route/v1/driving/") {
			t.Errorf("unexpected OSRM path %q", r.URL.Path)
		}
		w.Write([]byte(`{"code":"Ok","routes":[{"distance":5400.5,"duration":780.2}]}`))
	}))
	defer stub.Close()

	e := NewWith(stub.URL, stub.Client())
	a := match.Point{Lat: 26.6921, Lng: 67.6133}
	b := match.Point{Lat: 26.7300, Lng: 67.7769}

	est, err := e.Estimate(a, b)
	if err != nil || !est.FromOSRM {
		t.Fatalf("first estimate = %+v, %v; want OSRM provenance", est, err)
	}
	if est.Meters != 5400.5 || est.Seconds != 780.2 {
		t.Fatalf("estimate = %+v, want the stub's numbers", est)
	}

	// The same pair must hit the cache — one upstream call total.
	if _, err := e.Estimate(a, b); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("OSRM called %d times, want 1 (cache hit)", calls)
	}

	// Cache expiry: forcing the clock forward re-asks upstream.
	e.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	if _, err := e.Estimate(a, b); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("after TTL expiry OSRM called %d times, want 2", calls)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestOSRMFailureFallsBack(t *testing.T) {
	// A dead upstream must answer instantly from the fallback model.
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer stub.Close()

	e := NewWith(stub.URL, stub.Client())
	a := match.Point{Lat: 26.6921, Lng: 67.6133}
	b := match.Point{Lat: 26.6880, Lng: 67.6110}
	est, err := e.Estimate(a, b)
	if err != nil {
		t.Fatalf("fallback path must not error, got %v", err)
	}
	if est.FromOSRM {
		t.Fatal("after upstream failure the answer must be the fallback model")
	}
	if est.Meters <= 0 || est.Seconds <= 0 {
		t.Fatalf("fallback answer = %+v, want positive distance and time", est)
	}
}

func TestRoundingSharesCacheEntries(t *testing.T) {
	calls := 0
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"code":"Ok","routes":[{"distance":1000,"duration":120}]}`))
	}))
	defer stub.Close()

	e := NewWith(stub.URL, stub.Client())
	base := match.Point{Lat: 26.69210000, Lng: 67.61330000}
	near := match.Point{Lat: 26.69210004, Lng: 67.61330004} // sub-grid jitter
	if _, err := e.Estimate(base, match.Point{Lat: 26.7, Lng: 67.7}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Estimate(near, match.Point{Lat: 26.7, Lng: 67.7}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("same grid cell must share one cache entry, calls = %d", calls)
	}
}
