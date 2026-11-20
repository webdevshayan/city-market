package match

import (
	"errors"
	"math"
	"testing"
)

// The formula is checked against externally known great-circle distances,
// not against itself.
func TestHaversineAgainstKnownDistances(t *testing.T) {
	tests := []struct {
		name  string
		a, b  Point
		want  float64
		slack float64 // allowed relative slack
	}{
		{"London to Paris ≈ 343.5 km", Point{51.5074, -0.1278}, Point{48.8566, 2.3522}, 343_500, 0.01},
		{"one degree of longitude on the equator ≈ 111.19 km", Point{0, 0}, Point{0, 1}, 111_195, 0.005},
		{"one degree of latitude ≈ 111.19 km", Point{0, 0}, Point{1, 0}, 111_195, 0.005},
		{"identical points are zero", Point{24.86, 67.00}, Point{24.86, 67.00}, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := HaversineMeters(tc.a, tc.b)
			if math.Abs(got-tc.want) > tc.want*tc.slack {
				t.Fatalf("distance = %.0f m, want ≈ %.0f m", got, tc.want)
			}
		})
	}
}

func TestNearestSkipsInactive(t *testing.T) {
	origin := Point{24.8600, 67.0000}
	candidates := []Place{
		{ID: "far", Name: "Far Shop", Pos: Point{24.9000, 67.0000}, Active: true},
		{ID: "closest", Name: "Closed Shop", Pos: Point{24.8601, 67.0000}, Active: false},
		{ID: "near", Name: "Open Shop", Pos: Point{24.8650, 67.0000}, Active: true},
	}
	got, dist, err := Nearest(origin, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "near" {
		t.Fatalf("nearest = %q, want %q (inactive candidates must be skipped)", got.ID, "near")
	}
	if dist <= 0 || dist > 600 {
		t.Fatalf("distance = %.0f m, want a small positive number", dist)
	}
}

func TestNearestTieBreaksByID(t *testing.T) {
	origin := Point{0, 0}
	candidates := []Place{
		{ID: "bbb", Pos: Point{1, 0}, Active: true},
		{ID: "aaa", Pos: Point{1, 0}, Active: true}, // identical distance
	}
	got, _, err := Nearest(origin, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "aaa" {
		t.Fatalf("tie broken by %q, want %q (deterministic dispatch)", got.ID, "aaa")
	}
}

func TestNearestNoCandidate(t *testing.T) {
	candidates := []Place{{ID: "x", Pos: Point{1, 1}, Active: false}}
	if _, _, err := Nearest(Point{0, 0}, candidates); !errors.Is(err, ErrNoCandidate) {
		t.Fatalf("want ErrNoCandidate, got %v", err)
	}
	if _, _, err := Nearest(Point{0, 0}, nil); !errors.Is(err, ErrNoCandidate) {
		t.Fatalf("want ErrNoCandidate for empty list, got %v", err)
	}
}

func TestSortByDistanceDeterministic(t *testing.T) {
	origin := Point{24.86, 67.00}
	candidates := []Place{
		{ID: "b", Pos: Point{24.90, 67.00}, Active: true},
		{ID: "a", Pos: Point{24.87, 67.00}, Active: true},
		{ID: "c", Pos: Point{25.00, 67.00}, Active: false},
	}
	ranked := SortByDistance(origin, candidates)
	if ranked[0].ID != "a" || ranked[1].ID != "b" || ranked[2].ID != "c" {
		t.Fatalf("order = %v,%v,%v; want a,b,c (inactive ranked last, ties by ID)",
			ranked[0].ID, ranked[1].ID, ranked[2].ID)
	}
	if ranked[0].DistanceMeters >= ranked[1].DistanceMeters {
		t.Fatal("ranked distances must ascend")
	}
}
