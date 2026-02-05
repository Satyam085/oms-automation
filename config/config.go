package config

import "oms-automtion/models"

const (
	BaseURL  = "https://omsapi.geourja.com"
	PageSize = 10
	
	// Rate limiting delays (in milliseconds)
	DelayBetweenPages   = 1000  // 1 second between pagination requests
	DelayBetweenOutages = 2000  // 2 seconds between processing each outage
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
	{Label: "< 1 hour", MaxHours: 1, ReasonID: 25},     // No Cause found
	{Label: "1–4 hours", MaxHours: 4, ReasonID: 28},    // Relay Problems
	{Label: "4–8 hours", MaxHours: 8, ReasonID: 6},     // Cable Wire Fault
	{Label: "8–24 hours", MaxHours: 24, ReasonID: 30},  // Transformer Failure
	{Label: "> 24 hours", MaxHours: 9999, ReasonID: 9}, // Conductor Snapped HT Line
}
