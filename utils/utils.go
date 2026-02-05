package utils

import (
	"fmt"
	"strings"
	"time"

	"oms-automtion/models"
)

// CalculateDurationFromTimestamps calculates duration in hours from occurrence and restoration timestamps
func CalculateDurationFromTimestamps(occurAt, restoreAt string) (float64, error) {
	if occurAt == "" {
		return 0, fmt.Errorf("outage_occur_at is empty")
	}

	// Parse occurrence time (ISO 8601 / RFC3339 format: "2026-01-22T10:48:53.816Z")
	occurTime, err := time.Parse(time.RFC3339, occurAt)
	if err != nil {
		return 0, fmt.Errorf("parse outage_occur_at %q: %w", occurAt, err)
	}

	var restoreTime time.Time
	if restoreAt == "" {
		// Ongoing outage - use current time
		restoreTime = time.Now()
	} else {
		restoreTime, err = time.Parse(time.RFC3339, restoreAt)
		if err != nil {
			return 0, fmt.Errorf("parse outage_restore_at %q: %w", restoreAt, err)
		}
	}

	duration := restoreTime.Sub(occurTime)
	hours := duration.Hours()
	
	if hours < 0 {
		return 0, fmt.Errorf("negative duration: restore_at (%s) before occur_at (%s)", restoreAt, occurAt)
	}

	return hours, nil
}

// ParseDuration parses "HH:MM:SS.mmm" into total hours
func ParseDuration(raw string) (float64, error) {
	clean := strings.Split(raw, ".")[0] // strip millis
	t, err := time.Parse("15:04:05", clean)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", raw, err)
	}
	return float64(t.Hour()) + float64(t.Minute())/60.0 + float64(t.Second())/3600.0, nil
}

func ClassifyRule(hours float64, rules []models.DurationRule) models.DurationRule {
	for _, r := range rules {
		if hours <= r.MaxHours {
			return r
		}
	}
	if len(rules) > 0 {
		return rules[len(rules)-1]
	}
	return models.DurationRule{}
}
