package services_test

import (
	"testing"

	"coal-governance-backend/services"
)

func TestClassifySLAFallback(t *testing.T) {
	tests := []struct {
		name               string
		description        string
		expectedClass      string
		expectedSLAHours   int
	}{
		{
			name:             "Critical methane gas leak",
			description:      "Severe methane gas leak detected in Pit-4 face",
			expectedClass:    "CRITICAL_HAZARD",
			expectedSLAHours: 2,
		},
		{
			name:             "Critical roof fall / collapse",
			description:      "Roof fall and structural collapse near main incline",
			expectedClass:    "CRITICAL_HAZARD",
			expectedSLAHours: 2,
		},
		{
			name:             "Urgent ventilation defect",
			description:      "Auxiliary ventilation fan electrical tripping intermittently",
			expectedClass:    "URGENT",
			expectedSLAHours: 12,
		},
		{
			name:             "Urgent dust / PPE issue",
			description:      "Excess coal dust accumulation and missing PPE at transfer point",
			expectedClass:    "URGENT",
			expectedSLAHours: 12,
		},
		{
			name:             "Routine administrative grievance",
			description:      "Request for updated canteen shift allowance slip",
			expectedClass:    "ROUTINE",
			expectedSLAHours: 48,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class, hours := services.ClassifySLAFallback(tt.description)
			if class != tt.expectedClass {
				t.Errorf("expected class %s, got %s", tt.expectedClass, class)
			}
			if hours != tt.expectedSLAHours {
				t.Errorf("expected sla %d, got %d", tt.expectedSLAHours, hours)
			}
		})
	}
}
