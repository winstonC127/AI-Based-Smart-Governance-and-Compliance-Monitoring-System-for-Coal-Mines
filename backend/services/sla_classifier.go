package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// SLAClassificationResponse represents the response from the AI classification service.
type SLAClassificationResponse struct {
	Success        bool   `json:"success"`
	Classification string `json:"classification"`
	SLAHours       int    `json:"sla_hours"`
	Reasoning      string `json:"reasoning"`
}

// ClassifySLAWithAI classifies a violation or grievance description using the Gemini AI service.
// If the AI call fails or times out (3-second limit), it falls back to a deterministic keyword classifier.
func ClassifySLAWithAI(aiServiceURL string, description string) (string, int) {
	if strings.TrimSpace(description) == "" {
		return "ROUTINE", 48
	}

	payload := map[string]string{
		"description": description,
	}
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[SLA Classifier] JSON marshal error: %v. Using fallback.", err)
		return ClassifySLAFallback(description)
	}

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	targetURL := fmt.Sprintf("%s/ai/classify-severity", strings.TrimRight(aiServiceURL, "/"))
	resp, err := client.Post(targetURL, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		log.Printf("[SLA Classifier] AI service unreachable/timed out (%v). Using deterministic fallback.", err)
		return ClassifySLAFallback(description)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[SLA Classifier] AI service HTTP %d. Using fallback.", resp.StatusCode)
		return ClassifySLAFallback(description)
	}

	var res SLAClassificationResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		log.Printf("[SLA Classifier] Error decoding AI response: %v. Using fallback.", err)
		return ClassifySLAFallback(description)
	}

	if !res.Success || res.SLAHours <= 0 {
		return ClassifySLAFallback(description)
	}

	classification := res.Classification
	slaHours := res.SLAHours
	if slaHours != 2 && slaHours != 12 && slaHours != 48 {
		switch classification {
		case "CRITICAL_HAZARD":
			slaHours = 2
		case "URGENT":
			slaHours = 12
		default:
			slaHours = 48
		}
	}

	log.Printf("[SLA Classifier] AI Classified as %s (%d hrs) for: %s", classification, slaHours, utilsTruncate(description, 40))
	return classification, slaHours
}

// ClassifySLAFallback performs rule-based keyword classification when AI service is unavailable.
func ClassifySLAFallback(description string) (string, int) {
	lower := strings.ToLower(description)

	// Critical hazard keywords -> 2 hours
	criticalKeywords := []string{
		"gas leak", "methane", "fire", "smoke", "collapse", "roof fall", "explosion",
		"flood", "inundation", "trapped", "fatality", "critical hazard", "immediate danger",
		"toxic", "strata", "carbon monoxide", "ch4", "co2", "emergency", "blast",
	}
	for _, kw := range criticalKeywords {
		if strings.Contains(lower, kw) {
			return "CRITICAL_HAZARD", 2
		}
	}

	// Urgent keywords -> 12 hours
	urgentKeywords := []string{
		"ventilation", "dust", "crack", "electrical", "brake", "cable", "ppe",
		"machine fault", "urgent", "haul road", "overheating", "conveyor",
		"sensor failure", "leakage", "water accumulation", "overburden", "highwall",
	}
	for _, kw := range urgentKeywords {
		if strings.Contains(lower, kw) {
			return "URGENT", 12
		}
	}

	// Routine default -> 48 hours
	return "ROUTINE", 48
}

func utilsTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
