package config

import (
	"os"

	"oms-automtion/models"
)

const (
	BaseURL  = "https://omsapi.geourja.com"
	PageSize = 10

	// Rate limiting delays (in milliseconds)
	DelayBetweenPages   = 1000 // 1 second between pagination requests
	DelayBetweenOutages = 1000 // 1 seconds between processing each outage
)

// Creds holds OMS login credentials. Values are read from env vars at startup
// with fallbacks to baked-in defaults for local development.
var Creds = struct {
	CompanyName string
	EmpNo       string
	Password    string
	AppName     string
}{
	CompanyName: envOr("OMS_COMPANY_NAME", "DGVCL"),
	EmpNo:       envOr("OMS_EMP_NO", "25894"),
	Password:    envOr("OMS_PASSWORD", "Dgvcl@8949"),
	AppName:     envOr("OMS_APP_NAME", "SFMS-Web"),
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var DurationRules = []models.DurationRule{
	{Label: "≤ 15 min", MaxHours: 0.25, ReasonID: 21},      // Jumper Touching (0-15 minutes)
	{Label: "15 min–1 hr", MaxHours: 1, ReasonID: 20},      // Jumper Burnt (15 min - 1 hour)
	{Label: "1–3 hours", MaxHours: 3, ReasonID: 31},        // Tree / Tree Branch Falling (1-3 hours)
	{Label: "3–8 hours", MaxHours: 8, ReasonID: 9},         // Conductor Snapped HT Line (3-8 hours)
	{Label: "~15.73 hours", MaxHours: 15.73, ReasonID: 25}, // No Cause found (exactly 15.73 hours)
	// Any other duration will be skipped
}
