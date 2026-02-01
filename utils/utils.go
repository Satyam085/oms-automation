package utils

import (
	"fmt"
	"strings"
	"time"

	"oms-automtion/models"
)

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
