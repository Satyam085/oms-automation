package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"oms-automtion/models"
)

const (
	BaseURL  = "https://omsapi.geourja.com"
	PageSize = 10

	// Rate limiting delays (in milliseconds)
	DelayBetweenPages   = 1000 // 1 second between pagination requests
	DelayBetweenOutages = 1000 // 1 seconds between processing each outage
)

// Creds holds default OMS login credentials. Retained for backward compatibility.
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

func loadDotEnv() {
	file, err := os.Open(".env")
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `"'`)
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

// LoadProfiles scans environment variables and returns all configured user profiles.
// Supports single default profile (PASSCODE, OMS_EMP_NO, OMS_PASSWORD) as Profile 1,
// as well as indexed profiles (PASSCODE_2, OMS_EMP_NO_2, OMS_PASSWORD_2, etc.).
func LoadProfiles() []models.UserProfile {
	loadDotEnv()
	var profiles []models.UserProfile

	// Profile 1 (legacy / primary)
	p1Passcode := envOr("PASSCODE_1", os.Getenv("PASSCODE"))
	p1EmpNo := envOr("OMS_EMP_NO_1", os.Getenv("OMS_EMP_NO"))
	p1Password := envOr("OMS_PASSWORD_1", os.Getenv("OMS_PASSWORD"))
	p1Company := envOr("OMS_COMPANY_NAME_1", envOr("OMS_COMPANY_NAME", "DGVCL"))
	p1App := envOr("OMS_APP_NAME_1", envOr("OMS_APP_NAME", "SFMS-Web"))

	if p1Passcode == "" {
		p1Passcode = "123456"
	}
	if p1EmpNo == "" {
		p1EmpNo = "25894"
	}
	if p1Password == "" {
		p1Password = "Dgvcl@8949"
	}

	profiles = append(profiles, models.UserProfile{
		Name:        "Profile 1",
		Passcode:    p1Passcode,
		CompanyName: p1Company,
		EmpNo:       p1EmpNo,
		Password:    p1Password,
		AppName:     p1App,
	})

	// Additional profiles 2 through 10
	for i := 2; i <= 10; i++ {
		suffix := fmt.Sprintf("_%d", i)
		passcode := os.Getenv("PASSCODE" + suffix)
		empNo := os.Getenv("OMS_EMP_NO" + suffix)
		password := os.Getenv("OMS_PASSWORD" + suffix)
		company := envOr("OMS_COMPANY_NAME"+suffix, p1Company)
		app := envOr("OMS_APP_NAME"+suffix, p1App)

		if passcode != "" || empNo != "" {
			profiles = append(profiles, models.UserProfile{
				Name:        fmt.Sprintf("Profile %d", i),
				Passcode:    passcode,
				CompanyName: company,
				EmpNo:       empNo,
				Password:    password,
				AppName:     app,
			})
		}
	}

	return profiles
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var DurationRules = []models.DurationRule{
	{Label: "≤ 15 min", MaxHours: 0.25, ReasonID: 21, ReasonName: "Jumper Touching"},
	{Label: "15 min–1 hr", MaxHours: 1, ReasonID: 20, ReasonName: "Jumper Burnt"},
	{Label: "1–3 hours", MaxHours: 3, ReasonID: 31, ReasonName: "Tree / Tree Branch Falling"},
	{Label: "3–8 hours", MaxHours: 8, ReasonID: 74, ReasonName: "Conductor Snapped HT Line"},
	{Label: "~15.73 hours", MaxHours: 15.73, ReasonID: 28, ReasonName: "Relay Problems"},
	// Any other duration will be skipped
}
