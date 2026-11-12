package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"oms-automtion/models"
)

const BaseURL = "https://omsapi.geourja.com"

// PageSize is how many pending outages one /reason/pending call asks for
// (OMS_PAGE_SIZE, default 500). The call costs about the same whatever the
// limit — 8s for 10 outages, 9s for all 393 (measured 12 Nov 2026) — so the
// default takes the whole queue in one call instead of one call per 10 or 50.
// Pacing between requests is oms.Client.Gap.
func PageSize() int {
	if n, err := strconv.Atoi(os.Getenv("OMS_PAGE_SIZE")); err == nil && n > 0 {
		return n
	}
	return 500
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
	// Was #74 from 12 May 2026 — that is Cyclone in OMS, not Conductor Snapped.
	{Label: "3–8 hours", MaxHours: 8, ReasonID: 9, ReasonName: "Conductor Snapped HT Line"},
	// Over 8h: #28 Relay Problems for ~15h on KUMBHIYA/KHAKHAR, else #25 (see ruleFor).
}
