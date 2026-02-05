package config

import "oms-automtion/models"

const (
	BaseURL  = "https://omsapi.geourja.com"
	PageSize = 10
	
	// Rate limiting delays (in milliseconds)
	DelayBetweenPages   = 1000  // 1 second between pagination requests
	DelayBetweenOutages = 1000  // 1 seconds between processing each outage
)

var Creds = struct {
	CompanyName string
	EmpNo       string
	Password    string
	AppName     string
}{
	CompanyName: "DGVCL",
	EmpNo:       "25894",
	Password:    "Dgvcl@8949",
	AppName:     "SFMS-Web",
}

var DurationRules = []models.DurationRule{
	{Label: "≤ 15 min", MaxHours: 0.25, ReasonID: 21},      // Jumper Touching (0-15 minutes)
	{Label: "15 min–1 hr", MaxHours: 1, ReasonID: 20},      // Jumper Burnt (15 min - 1 hour)
	{Label: "1–3 hours", MaxHours: 3, ReasonID: 31},        // Tree / Tree Branch Falling (1-3 hours)
	{Label: "3–8 hours", MaxHours: 8, ReasonID: 9},         // Conductor Snapped HT Line (3-8 hours)
	{Label: "~15.73 hours", MaxHours: 15.73, ReasonID: 25}, // No Cause found (exactly 15.73 hours)
	// Any other duration will be skipped
}
