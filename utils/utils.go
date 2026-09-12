package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"oms-automtion/models"
)

// CalculateDurationFromTimestamps calculates duration in hours
// from separate date and time strings (e.g. "2026-01-28" + "17:37:25.743")
func CalculateDurationFromTimestamps(occurDate, occurTime, restoreDate, restoreTime string) (float64, error) {
	occurDate = strings.TrimSpace(occurDate)
	occurTime = strings.TrimSpace(occurTime)
	restoreDate = strings.TrimSpace(restoreDate)
	restoreTime = strings.TrimSpace(restoreTime)

	if occurDate == "" || occurTime == "" {
		return 0, fmt.Errorf("outage occur date/time is empty")
	}

	// Strip sub-second precision for simpler parsing
	occurTimeClean := strings.Split(occurTime, ".")[0]
	occurStr := occurDate + " " + occurTimeClean

	// Parse in time.Local (forced to IST in main) so the time.Now() fallback
	// below compares against the same frame.
	const layout = "2006-01-02 15:04:05"
	occurParsed, err := time.ParseInLocation(layout, occurStr, time.Local)
	if err != nil {
		return 0, fmt.Errorf("parse occur %q: %w", occurStr, err)
	}

	var restoreParsed time.Time
	if restoreDate == "" || restoreTime == "" {
		restoreParsed = time.Now()
	} else {
		restoreTimeClean := strings.Split(restoreTime, ".")[0]
		restoreStr := restoreDate + " " + restoreTimeClean
		restoreParsed, err = time.ParseInLocation(layout, restoreStr, time.Local)
		if err != nil {
			return 0, fmt.Errorf("parse restore %q: %w", restoreStr, err)
		}
	}

	dur := restoreParsed.Sub(occurParsed)
	if dur < 0 {
		return 0, fmt.Errorf("negative duration: restore before occur")
	}

	return dur.Hours(), nil
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

	// Nothing matched: the last rule is a narrow window, not an upper bound, so
	// falling back to it would mis-assign. Callers (see ruleFor) handle the
	// over-range case themselves.
	return models.DurationRule{}
}
