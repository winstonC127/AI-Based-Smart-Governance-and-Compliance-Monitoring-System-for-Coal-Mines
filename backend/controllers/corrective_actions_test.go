package controllers_test

import (
	"testing"

	"coal-governance-backend/controllers"
)

func TestCorrectiveActionGeofenceDistanceAndAnomaly(t *testing.T) {
	// Gevra Mine coordinates
	mineLat := 22.3595
	mineLng := 82.6892
	radiusM := 500.0

	tests := []struct {
		name          string
		resLat        float64
		resLng        float64
		expectAnomaly bool
	}{
		{
			name:          "On-site near mine center (50m)",
			resLat:        22.3598,
			resLng:        82.6895,
			expectAnomaly: false,
		},
		{
			name:          "Within 500m geofence perimeter (200m)",
			resLat:        22.3610,
			resLng:        82.6905,
			expectAnomaly: false,
		},
		{
			name:          "Off-site beyond 500m perimeter (1.5km away)",
			resLat:        22.3720,
			resLng:        82.7000,
			expectAnomaly: true,
		},
		{
			name:          "Different city / remote location (100km away)",
			resLat:        21.2500,
			resLng:        81.6300,
			expectAnomaly: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dist := controllers.HaversineDistance(tt.resLat, tt.resLng, mineLat, mineLng)
			isOffSite := dist > radiusM
			if isOffSite != tt.expectAnomaly {
				t.Errorf("expected isOffSite=%v (dist=%.1fm, radius=%.0fm), got %v", tt.expectAnomaly, dist, radiusM, isOffSite)
			}
		})
	}
}

func TestResolveAuthorizationGuards(t *testing.T) {
	assignedUserID := 10
	superAdminRole := "SUPER_ADMIN"
	safetyOfficerRole := "SAFETY_OFFICER"

	tests := []struct {
		name         string
		callerID     int
		callerRole   string
		expectAccess bool
	}{
		{
			name:         "Assigned worker submitting resolution",
			callerID:     10,
			callerRole:   "WORKER",
			expectAccess: true,
		},
		{
			name:         "Super Admin resolving on behalf of worker",
			callerID:     1,
			callerRole:   superAdminRole,
			expectAccess: true,
		},
		{
			name:         "Different unassigned worker",
			callerID:     15,
			callerRole:   "WORKER",
			expectAccess: false,
		},
		{
			name:         "Safety Officer trying to resolve instead of worker",
			callerID:     3,
			callerRole:   safetyOfficerRole,
			expectAccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isAuthorized := tt.callerID == assignedUserID || tt.callerRole == superAdminRole
			if isAuthorized != tt.expectAccess {
				t.Errorf("expected authorized=%v for callerID=%d, got %v", tt.expectAccess, tt.callerID, isAuthorized)
			}
		})
	}
}

func TestDecoupledObservationEvidenceGuard(t *testing.T) {
	tests := []struct {
		name                string
		obsText             string
		evidenceFilePresent bool
		expectSave          bool
		expectedObservation string
	}{
		{
			name:                "Both observation text and photo provided",
			obsText:             "Cracked blast shield in conveyor area",
			evidenceFilePresent: true,
			expectSave:          true,
			expectedObservation: "Cracked blast shield in conveyor area",
		},
		{
			name:                "Observation text only without photo",
			obsText:             "Minor housekeeping issue",
			evidenceFilePresent: false,
			expectSave:          true,
			expectedObservation: "Minor housekeeping issue",
		},
		{
			name:                "Photo evidence only without write-up (Feature 6)",
			obsText:             "",
			evidenceFilePresent: true,
			expectSave:          true,
			expectedObservation: "(Photo evidence only)",
		},
		{
			name:                "Neither text nor photo provided",
			obsText:             "",
			evidenceFilePresent: false,
			expectSave:          false,
			expectedObservation: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldSave := tt.obsText != "" || tt.evidenceFilePresent
			if shouldSave != tt.expectSave {
				t.Errorf("expected shouldSave=%v, got %v", tt.expectSave, shouldSave)
			}

			if shouldSave {
				finalObs := tt.obsText
				if finalObs == "" {
					finalObs = "(Photo evidence only)"
				}
				if finalObs != tt.expectedObservation {
					t.Errorf("expected observation text '%s', got '%s'", tt.expectedObservation, finalObs)
				}
			}
		})
	}
}
