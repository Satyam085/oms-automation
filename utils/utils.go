package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"oms-automtion/models"
)

// CalculateDurationFromTimestamps calculates duration in hours
// from RFC3339 timestamps (e.g. "2026-01-22T10:48:53.816Z")
func CalculateDurationFromTimestamps(occurAt, restoreAt string) (float64, error) {
	occurAt = strings.TrimSpace(occurAt)
	restoreAt = strings.TrimSpace(restoreAt)

	fmt.Printf("[DEBUG] occurAt=%q restoreAt=%q\n", occurAt, restoreAt)

	if occurAt == "" {
		return 0, fmt.Errorf("outage_occur_at is empty")
	}

	occurTime, err := time.Parse(time.RFC3339, occurAt)
	if err != nil {
		fmt.Printf("[DEBUG] occurAt parse failed: %v\n", err)
		return 0, fmt.Errorf("parse outage_occur_at %q: %w", occurAt, err)
	}

	var restoreTime time.Time
	if restoreAt == "" {
		fmt.Println("[DEBUG] restoreAt empty -> using time.Now()")
		restoreTime = time.Now().UTC()
	} else {
		restoreTime, err = time.Parse(time.RFC3339, restoreAt)
		if err != nil {
			fmt.Printf("[DEBUG] restoreAt parse failed: %v\n", err)
			return 0, fmt.Errorf("parse outage_restore_at %q: %w", restoreAt, err)
		}
	}

	fmt.Printf("[DEBUG] occurTime=%v restoreTime=%v\n", occurTime, restoreTime)

	duration := restoreTime.Sub(occurTime)

	fmt.Printf("[DEBUG] raw duration=%v\n", duration)

	if duration < 0 {
		return 0, fmt.Errorf(
			"negative duration: restore_at (%s) before occur_at (%s)",
			restoreAt,
			occurAt,
		)
	}

	hours := duration.Hours()

	fmt.Printf("[DEBUG] calculated hours=%.4f\n", hours)

	return hours, nil
}

// ParseDuration parses duration string "HH:MM:SS" or "HH:MM:SS.mmm"
// into total hours (supports > 24 hours)
func ParseDuration(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return 0, fmt.Errorf("duration is empty")
	}

	// Strip milliseconds if present
	clean := strings.Split(raw, ".")[0]

	parts := strings.Split(clean, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid duration format: %q", raw)
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid hours in %q: %w", raw, err)
	}

	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid minutes in %q: %w", raw, err)
	}

	seconds, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("invalid seconds in %q: %w", raw, err)
	}

	if minutes < 0 || minutes >= 60 || seconds < 0 || seconds >= 60 {
		return 0, fmt.Errorf("invalid time range in %q", raw)
	}

	totalHours :=
		float64(hours) +
			float64(minutes)/60.0 +
			float64(seconds)/3600.0

	return totalHours, nil
}

// ClassifyRule returns the first matching rule based on MaxHours
func ClassifyRule(hours float64, rules []models.DurationRule) models.DurationRule {
	for _, r := range rules {
		if hours <= r.MaxHours {
			return r
		}
	}

	// Return last rule if nothing matched
	if len(rules) > 0 {
		return rules[len(rules)-1]
	}

	return models.DurationRule{}
}