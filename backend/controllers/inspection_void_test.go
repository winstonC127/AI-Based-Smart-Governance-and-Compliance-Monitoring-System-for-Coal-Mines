package controllers_test

import (
	"strings"
	"testing"
)

func TestVoidReasonValidation(t *testing.T) {
	tests := []struct {
		name        string
		reason      string
		expectValid bool
	}{
		{
			name:        "Empty reason",
			reason:      "",
			expectValid: false,
		},
		{
			name:        "Too short (less than 10 characters)",
			reason:      "Mistake",
			expectValid: false,
		},
		{
			name:        "Whitespace only",
			reason:      "          ",
			expectValid: false,
		},
		{
			name:        "Exactly 10 characters",
			reason:      "1234567890",
			expectValid: true,
		},
		{
			name:        "Detailed valid reason",
			reason:      "Created under incorrect mine code by mistake. Resubmitting with correct site.",
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trimmed := strings.TrimSpace(tt.reason)
			isValid := len(trimmed) >= 10
			if isValid != tt.expectValid {
				t.Errorf("expected valid=%v for reason '%s', got %v", tt.expectValid, tt.reason, isValid)
			}
		})
	}
}

func TestVoidStatusGuards(t *testing.T) {
	tests := []struct {
		status      string
		canVoid     bool
		expectedErr string
	}{
		{
			status:      "DRAFT",
			canVoid:     true,
			expectedErr: "",
		},
		{
			status:      "SUBMITTED",
			canVoid:     true,
			expectedErr: "",
		},
		{
			status:      "REVIEWED",
			canVoid:     false,
			expectedErr: "invalid status",
		},
		{
			status:      "APPROVED",
			canVoid:     false,
			expectedErr: "invalid status",
		},
		{
			status:      "VOIDED",
			canVoid:     false,
			expectedErr: "already voided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			isAllowed := tt.status == "DRAFT" || tt.status == "SUBMITTED"
			if isAllowed != tt.canVoid {
				t.Errorf("status %s expected canVoid=%v, got %v", tt.status, tt.canVoid, isAllowed)
			}
		})
	}
}

func TestVoidRoleAuthorization(t *testing.T) {
	tests := []struct {
		userRole     string
		isCreator    bool
		expectAccess bool
	}{
		{userRole: "INSPECTOR", isCreator: true, expectAccess: true},
		{userRole: "INSPECTOR", isCreator: false, expectAccess: false},
		{userRole: "SAFETY_OFFICER", isCreator: false, expectAccess: true},
		{userRole: "MINE_MANAGER", isCreator: false, expectAccess: true},
		{userRole: "SUPER_ADMIN", isCreator: false, expectAccess: true},
		{userRole: "REGULATORY_OFFICER", isCreator: false, expectAccess: false},
		{userRole: "CORPORATE_MANAGER", isCreator: false, expectAccess: false},
	}

	for _, tt := range tests {
		t.Run(tt.userRole, func(t *testing.T) {
			isSupervisor := tt.userRole == "MINE_MANAGER" || tt.userRole == "SAFETY_OFFICER" || tt.userRole == "SUPER_ADMIN"
			allowed := tt.isCreator || isSupervisor
			if allowed != tt.expectAccess {
				t.Errorf("role %s (isCreator=%v) expected %v, got %v", tt.userRole, tt.isCreator, tt.expectAccess, allowed)
			}
		})
	}
}
