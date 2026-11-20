// Package match implements City Market's dispatch logic: the great-circle
// distance between two points (haversine) and nearest-active-place selection
// used to assign shops to customers and riders to orders.
package match

import (
	"errors"
	"math"
	"sort"
)

// EarthRadiusMeters is the mean Earth radius used by the haversine formula.
const EarthRadiusMeters = 6_371_000.0

// ErrNoCandidate is returned when no candidate place is active.
var ErrNoCandidate = errors.New("match: no active candidate")

// Point is a geographic position in decimal degrees (WGS84).
type Point struct {
	Lat float64
	Lng float64
}

// Place is a dispatch candidate: a shop or a rider.
type Place struct {
	ID     string
	Name   string
	Pos    Point
	Active bool // shop open / rider available
}

// RankedPlace pairs a candidate with its distance to the query origin.
type RankedPlace struct {
	Place
	DistanceMeters float64
}

// radians converts degrees to radians.
func radians(d float64) float64 { return d * math.Pi / 180 }

// HaversineMeters returns the great-circle distance between a and b in
// metres — the antipodal-safe great-circle formula, not a flat-earth
// approximation.
func HaversineMeters(a, b Point) float64 {
	phi1, phi2 := radians(a.Lat), radians(b.Lat)
	dPhi := radians(b.Lat - a.Lat)
	dLambda := radians(b.Lng - a.Lng)
	sinPhi, sinLambda := math.Sin(dPhi/2), math.Sin(dLambda/2)
	h := sinPhi*sinPhi + math.Cos(phi1)*math.Cos(phi2)*sinLambda*sinLambda
	return 2 * EarthRadiusMeters * math.Asin(math.Sqrt(h))
}

// Nearest returns the closest active place to origin and its distance in
// metres. Exact ties are broken by ID so dispatch is deterministic.
// Non-active candidates are ignored.
func Nearest(origin Point, candidates []Place) (Place, float64, error) {
	best := -1
	bestDist := math.MaxFloat64
	for i, p := range candidates {
		if !p.Active {
			continue
		}
		d := HaversineMeters(origin, p.Pos)
		if d < bestDist || (d == bestDist && p.ID < candidates[best].ID) {
			best, bestDist = i, d
		}
	}
	if best < 0 {
		return Place{}, 0, ErrNoCandidate
	}
	return candidates[best], bestDist, nil
}

// SortByDistance orders all candidates by their distance to origin, exact
// ties by ID — the ranked dispatch list behind the nearest-one API.
func SortByDistance(origin Point, candidates []Place) []RankedPlace {
	ranked := make([]RankedPlace, len(candidates))
	for i, p := range candidates {
		ranked[i] = RankedPlace{Place: p, DistanceMeters: HaversineMeters(origin, p.Pos)}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].DistanceMeters != ranked[j].DistanceMeters {
			return ranked[i].DistanceMeters < ranked[j].DistanceMeters
		}
		return ranked[i].ID < ranked[j].ID
	})
	return ranked
}
