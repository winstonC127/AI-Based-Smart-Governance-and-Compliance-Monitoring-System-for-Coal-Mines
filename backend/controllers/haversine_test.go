package controllers_test

import (
	"math"
	"testing"

	"coal-governance-backend/controllers"
)

func TestHaversineDistance(t *testing.T) {
	// Gevra Mine: 22.3595, 82.6892
	// Point ~100m away
	lat1, lon1 := 22.3595, 82.6892
	lat2, lon2 := 22.3601, 82.6898

	dist := controllers.HaversineDistance(lat1, lon1, lat2, lon2)
	if dist <= 0 || dist > 200 {
		t.Errorf("expected distance between 0 and 200m, got %f", dist)
	}

	// Identical points should have distance ~ 0
	zeroDist := controllers.HaversineDistance(lat1, lon1, lat1, lon1)
	if zeroDist > 0.001 {
		t.Errorf("expected 0 distance, got %f", zeroDist)
	}

	// Gevra to Kusmunda (~12 km)
	// Kusmunda: 22.3167, 82.5833
	kusmundaLat, kusmundaLon := 22.3167, 82.5833
	distToKusmunda := controllers.HaversineDistance(lat1, lon1, kusmundaLat, kusmundaLon)
	distKm := distToKusmunda / 1000.0
	if distKm < 10.0 || distKm > 15.0 {
		t.Errorf("expected distance ~10-15km, got %f km", distKm)
	}

	// Test velocity calculation: 12 km in 5 minutes (0.0833 hours) -> 144 km/h (> 80 km/h anomaly)
	elapsedHours := 5.0 / 60.0
	speedKmh := distKm / elapsedHours
	if speedKmh <= 80.0 {
		t.Errorf("expected speed > 80 km/h, got %f", speedKmh)
	}
}

func TestHaversineSymmetry(t *testing.T) {
	lat1, lon1 := 22.3595, 82.6892
	lat2, lon2 := 20.9500, 85.2167

	d1 := controllers.HaversineDistance(lat1, lon1, lat2, lon2)
	d2 := controllers.HaversineDistance(lat2, lon2, lat1, lon1)

	if math.Abs(d1-d2) > 0.001 {
		t.Errorf("haversine must be symmetric: d1=%f, d2=%f", d1, d2)
	}
}
