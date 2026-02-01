package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// ─── CONFIG ──────────────────────────────────────────────────────────────────

const (
	baseURL  = "https://omsapi.geourja.com"
	pageSize = 10
)

// ── Credentials — update these ──
var creds = struct {
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

// token is populated at runtime by login()
var token string

// ─── DURATION → REASON MAPPING ───────────────────────────────────────────────
// Available reason IDs from the API (reasonData):
//
//	 1  Pin Puncture          |  2  Pole collapse
//	 4  Animal Fault          |  5  Bird Fault
//	 6  Cable Wire Fault      |  7  Vehicle accident
//	 8  Conductor Slipped     |  9  Conductor Snapped HT Line
//	10  Disaster/heavy rain   | 12  Flash over of Breakers
//	15  Guarding Fault        | 16  Hoarding Fallen
//	17  HT Connection Fault   | 18  Insulation Burnt
//	19  Insulator Puncture    | 20  Jumper Burnt
//	21  Jumper Touching       | 22  LA Fault
//	23  Lightening Stroke     | 24  Low Clearance at Crossing
//	25  No Cause found        | 27  Overhead ABC conductor fault
//	28  Relay Problems        | 29  Shakle Puncture
//	30  Transformer Failure   | 31  Tree / Tree Branch Falling
//	32  Underground cable     | 33  Underground cable by Outsider
//	65  Smoke                 | 74  Cyclone
//	75  Accident              | 78  Jumper
//	86  Danger to life        | 87  Bomb blast
//	88  Air Strike            | 91  Fire in buildings
//	92  Fire in godown        | 93  Fire in fields/jungle
//	100 DO Fuse short         | 112 Line Fabrication Damage
//	113 Heavy Wind
//
// ── UPDATE the ReasonID values below to match your actual logic ──
type DurationRule struct {
	Label    string
	MaxHours float64
	ReasonID int
}

var durationRules = []DurationRule{
	{Label: "< 1 hour", MaxHours: 1, ReasonID: 25},     // No Cause found
	{Label: "1–4 hours", MaxHours: 4, ReasonID: 28},    // Relay Problems
	{Label: "4–8 hours", MaxHours: 8, ReasonID: 6},     // Cable Wire Fault
	{Label: "8–24 hours", MaxHours: 24, ReasonID: 30},  // Transformer Failure
	{Label: "> 24 hours", MaxHours: 9999, ReasonID: 9}, // Conductor Snapped HT Line
}

// ─── STRUCTS: Pending outages ────────────────────────────────────────────────

type FilteredData struct {
	ID        int    `json:"id"`
	Field     string `json:"field"`
	Type      string `json:"type"`
	Operator  string `json:"operator"`
	Value     string `json:"value"`
	FromValue string `json:"fromValue"`
	ToValue   string `json:"toValue"`
}

type PendingRequest struct {
	OrderBy      *string        `json:"orderBy"`
	Order        *string        `json:"order"`
	FilteredData []FilteredData `json:"filteredData"`
	Offset       int            `json:"offset"`
	Limit        int            `json:"limit"`
}

type Outage struct {
	ID              string `json:"id"`
	InterruptionType string `json:"interruption_type"`
	OutageType      int    `json:"outage_type"`
	OutageTypeName  string `json:"outage_type_name"`
	FeederID        int    `json:"feeder_id"`
	FeederName      string `json:"feeder_name"`
	FeederCategory  string `json:"feeder_category"`
	OutageOccurAt   string `json:"outage_occur_at"`
	OutageRestoreAt string `json:"outage_restore_at"`
	Duration        string `json:"duration"`
	SSName          string `json:"ss_name"`
}

type PendingResponse struct {
	TotalRecords int      `json:"total_records"`
	Data         []Outage `json:"data"`
}

// ─── STRUCTS: Reason detail (GET /reason/outage/{id}) ────────────────────────
// Response nesting: data.feederPointGeoJson[0][0].row_to_json.features[*].properties.id

type GeoFeatureProperties struct {
	Hlt string `json:"hlt"`
	ID  int    `json:"id"` // ← this is the loc_id we need
}

type GeoFeatureGeometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

type GeoFeature struct {
	Type       string               `json:"type"`
	Geometry   GeoFeatureGeometry   `json:"geometry"`
	LocStr     string               `json:"loc_str"`
	Properties GeoFeatureProperties `json:"properties"`
}

type GeoFeatureCollection struct {
	Type     string       `json:"type"`
	Features []GeoFeature `json:"features"`
}

type RowToJSONWrapper struct {
	RowToJSON GeoFeatureCollection `json:"row_to_json"`
}

type ReasonDetailResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		OutageData         json.RawMessage         `json:"outageData"`
		FeederPointGeoJson [][]RowToJSONWrapper    `json:"feederPointGeoJson"`
	} `json:"data"`
}

// ─── STRUCTS: Submit reason (POST /reason/outage/{id}) ───────────────────────

type ReasonPayloadItem struct {
	LocID    int `json:"loc_id"`
	ReasonID int `json:"reason_id"`
}

// ─── STRUCTS: Login ──────────────────────────────────────────────────────────

type LoginRequest struct {
	CompanyName string `json:"companyName"`
	EmpNo       string `json:"empNo"`
	Password    string `json:"password"`
	AppName     string `json:"appName"`
}

type LoginResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    struct {
		Token string `json:"token"` // bearer token returned on success
	} `json:"data"`
}

// ─── HELPERS ─────────────────────────────────────────────────────────────────

// login authenticates and sets the package-level `token` variable.
// Must be called before any other API call.
func login() error {
	payload := LoginRequest{
		CompanyName: creds.CompanyName,
		EmpNo:       creds.EmpNo,
		Password:    creds.Password,
		AppName:     creds.AppName,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal login: %w", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/auth/login", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}
	// Login sends "bearer null" initially — no valid token yet
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("Authorization", "bearer null")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Referer", "https://smartoms.geourja.com/")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("login HTTP: %w", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// DEBUG — remove after fixing
	log.Printf("  [DEBUG] Login request body: %s", string(body))
	log.Printf("  [DEBUG] Login status code: %d", resp.StatusCode)
	log.Printf("  [DEBUG] Login raw response: %s", string(respBody))

	if resp.StatusCode != 200 {
		return fmt.Errorf("login returned %d: %s", resp.StatusCode, string(respBody))
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(respBody, &loginResp); err != nil {
		return fmt.Errorf("unmarshal login response: %w", err)
	}

	if !loginResp.Status || loginResp.Data.Token == "" {
		return fmt.Errorf("login failed: %s", loginResp.Message)
	}

	token = loginResp.Data.Token
	log.Printf("  ✓ Logged in as empNo=%s | token=%s...%s",
		creds.EmpNo, token[:10], token[len(token)-8:])
	return nil
}

// parseDuration parses "HH:MM:SS.mmm" into total hours
func parseDuration(raw string) (float64, error) {
	clean := strings.Split(raw, ".")[0] // strip millis
	t, err := time.Parse("15:04:05", clean)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", raw, err)
	}
	return float64(t.Hour()) + float64(t.Minute())/60.0 + float64(t.Second())/3600.0, nil
}

func classifyRule(hours float64) DurationRule {
	for _, r := range durationRules {
		if hours <= r.MaxHours {
			return r
		}
	}
	return durationRules[len(durationRules)-1]
}

// newAPIRequest builds an http.Request with all required OMS headers
func newAPIRequest(method, url string, body []byte) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "bearer "+token)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Referer", "https://smartoms.geourja.com/")
	return req, nil
}

// ─── STEP 1: Fetch all pending outages (paginated) ──────────────────────────

func fetchPendingOutages() ([]Outage, error) {
	var all []Outage
	offset := 0

	for {
		payload := PendingRequest{
			FilteredData: []FilteredData{{
				ID: 14, Field: "outage_occur_at", Type: "date",
				Operator:  "between",
				Value:     "2026-01-02 to 2026-02-01",
				FromValue: "2026-01-02",
				ToValue:   "2026-02-01",
			}},
			Offset: offset,
			Limit:  pageSize,
		}

		body, _ := json.Marshal(payload)
		req, err := newAPIRequest("POST", baseURL+"/reason/pending", body)
		if err != nil {
			return nil, err
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch pending: %w", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("pending returned %d: %s", resp.StatusCode, respBody)
		}

		var pr PendingResponse
		if err := json.Unmarshal(respBody, &pr); err != nil {
			return nil, fmt.Errorf("unmarshal pending: %w", err)
		}

		all = append(all, pr.Data...)
		log.Printf("  [Fetch] offset=%d got=%d total=%d", offset, len(pr.Data), pr.TotalRecords)

		if offset+pageSize >= pr.TotalRecords || len(pr.Data) == 0 {
			break
		}
		offset += pageSize
	}
	return all, nil
}

// ─── STEP 2: Extract loc_ids from the GeoJSON response ──────────────────────

func fetchLocIDs(outageID string) ([]int, error) {
	url := fmt.Sprintf("%s/reason/outage/%s", baseURL, outageID)

	req, err := newAPIRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch detail %s: %w", outageID, err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("detail %s returned %d: %s", outageID, resp.StatusCode, respBody)
	}

	var detail ReasonDetailResponse
	if err := json.Unmarshal(respBody, &detail); err != nil {
		return nil, fmt.Errorf("unmarshal detail %s: %w", outageID, err)
	}

	// Walk: feederPointGeoJson[*][*].row_to_json.features[*].properties.id
	var locIDs []int
	for _, outer := range detail.Data.FeederPointGeoJson {
		for _, inner := range outer {
			for _, feat := range inner.RowToJSON.Features {
				if feat.Properties.ID != 0 {
					locIDs = append(locIDs, feat.Properties.ID)
				}
			}
		}
	}
	return locIDs, nil
}

// ─── STEP 3: POST the reason ─────────────────────────────────────────────────

func submitReason(outageID string, locID int, reasonID int) error {
	url := fmt.Sprintf("%s/reason/outage/%s", baseURL, outageID)

	payload := []ReasonPayloadItem{{LocID: locID, ReasonID: reasonID}}
	body, _ := json.Marshal(payload)

	req, err := newAPIRequest("POST", url, body)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("submit %s: %w", outageID, err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("submit %s returned %d: %s", outageID, resp.StatusCode, respBody)
	}
	return nil
}

// ─── MAIN ────────────────────────────────────────────────────────────────────

func main() {
	rand.Seed(time.Now().UnixNano())
	log.Println("═══ OMS Outage Reason Automation ═══")

	// ── Step 0: Login & get token ──
	log.Println("\n[Step 0] Logging in...")
	if err := login(); err != nil {
		log.Fatalf("FATAL: login failed: %v", err)
	}

	// ── Step 1 ──
	log.Println("\n[Step 1] Fetching pending outages...")
	outages, err := fetchPendingOutages()
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}
	log.Printf("  → Fetched %d outages\n", len(outages))

	// ── Parse & classify durations ──
	type processedOutage struct {
		Outage        Outage
		DurationHours float64
		Rule          DurationRule
	}

	var processed []processedOutage
	for _, o := range outages {
		hours, err := parseDuration(o.Duration)
		if err != nil {
			log.Printf("  [WARN] Skip %s: %v", o.ID, err)
			continue
		}
		processed = append(processed, processedOutage{
			Outage:        o,
			DurationHours: hours,
			Rule:          classifyRule(hours),
		})
	}

	// ── Print summary table ──
	fmt.Println()
	fmt.Println("┌────────────────┬─────────────────┬────────┬────────────────┬──────────────────┬──────────┐")
	fmt.Println("│ Outage ID      │ Duration        │ Hours  │ Bucket         │ Feeder           │ ReasonID │")
	fmt.Println("├────────────────┼─────────────────┼────────┼────────────────┼──────────────────┼──────────┤")
	for _, p := range processed {
		fmt.Printf("│ %-14s │ %-15s │ %5.2f  │ %-14s │ %-16s │ %-8d │\n",
			p.Outage.ID, p.Outage.Duration, p.DurationHours,
			p.Rule.Label, p.Outage.FeederName, p.Rule.ReasonID)
	}
	fmt.Println("└────────────────┴─────────────────┴────────┴────────────────┴──────────────────┴──────────┘")

	// ── Steps 2 & 3: fetch loc_id → submit reason ──
	log.Printf("\n[Step 2 & 3] Processing %d outages...\n", len(processed))

	successCount := 0
	failCount := 0

	for i, p := range processed {
		id := p.Outage.ID
		log.Printf("  [%d/%d] Outage %s | %.2fh | reason_id=%d",
			i+1, len(processed), id, p.DurationHours, p.Rule.ReasonID)

		// Fetch available loc_ids from feederPointGeoJson
		locIDs, err := fetchLocIDs(id)
		if err != nil {
			log.Printf("    ✗ loc_ids fetch failed: %v", err)
			failCount++
			continue
		}
		if len(locIDs) == 0 {
			log.Printf("    ✗ No loc_ids in GeoJSON")
			failCount++
			continue
		}

		// Pick random loc_id
		pickedLocID := locIDs[rand.Intn(len(locIDs))]
		log.Printf("    → loc_id=%d (picked from %d poles)", pickedLocID, len(locIDs))

		// // Submit
		// if err := submitReason(id, pickedLocID, p.Rule.ReasonID); err != nil {
		// 	log.Printf("    ✗ Submit failed: %v", err)
		// 	failCount++
		// 	continue
		// }

		// log.Printf("    ✓ Submitted")
		// successCount++

		// Throttle to avoid rate limiting
		time.Sleep(300 * time.Millisecond)
	}

	// ── Summary ──
	fmt.Println()
	fmt.Println("─── Results ───")
	fmt.Printf("  Total:   %d\n", len(processed))
	fmt.Printf("  Success: %d\n", successCount)
	fmt.Printf("  Failed:  %d\n", failCount)
	log.Println("\n═══ Done ═══")
}