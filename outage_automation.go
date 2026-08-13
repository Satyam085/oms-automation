package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"oms-automtion/config"
	"oms-automtion/models"
	"oms-automtion/oms"
	"oms-automtion/utils"
)

/*
╔════════════════════════════════════════════════════════════════════════════╗
║                         REASON ID REFERENCE TABLE                          ║
╠═════╦══════════════════════════════════════════════════════════════════════╣
║ ID  ║ Reason Name                                                          ║
╠═════╬══════════════════════════════════════════════════════════════════════╣
║  1  ║ Pin Puncture                                                         ║
║  2  ║ Pole collapse                                                        ║
║  4  ║ Animal Fault                                                         ║
║  5  ║ Bird Fault                                                           ║
║  6  ║ Cable Wire Fault                                                     ║
║  7  ║ Vehicle accident                                                     ║
║  8  ║ Conductor Slipped From Pin Insulator                                 ║
║  9  ║ Conductor Snapped HT Line                                            ║
║ 10  ║ Disaster like heavy rain                                             ║
║ 12  ║ Flash over of Breakers                                               ║
║ 15  ║ Guarding Fault                                                       ║
║ 16  ║ Hoarding Fallen                                                      ║
║ 17  ║ HT Connection Internal Fault                                         ║
║ 18  ║ Insulation Burnt                                                     ║
║ 19  ║ Insulator Puncture                                                   ║
║ 20  ║ Jumper Burnt                                                         ║
║ 21  ║ Jumper Touching                                                      ║
║ 22  ║ LA Fault                                                             ║
║ 23  ║ Lightening Stroke                                                    ║
║ 24  ║ Low Clearance at Crossing                                            ║
║ 25  ║ No Cause found                                                       ║
║ 27  ║ Overhead ABC conductor fault                                         ║
║ 28  ║ Relay Problems                                                       ║
║ 29  ║ Shakle Puncture                                                     ║
║ 30  ║ Transformer Failure                                                  ║
║ 31  ║ Tree / Tree Branch Falling                                           ║
║ 32  ║ Under ground cable fault                                             ║
║ 33  ║ Under Ground Cable Fault by Outsider                                 ║
║ 65  ║ Smoke                                                                ║
║ 74  ║ Cyclone                                                              ║
║ 75  ║ Accident                                                             ║
║ 78  ║ Jumper                                                               ║
║ 86  ║ Danger to life                                                       ║
║ 87  ║ Bomb blast                                                           ║
║ 88  ║ Air Strike                                                           ║
║ 91  ║ Fire in buildings                                                    ║
║ 92  ║ Fire in godown                                                       ║
║ 93  ║ Fire in fiels/jungle                                                 ║
║ 100 ║ DO Fuse short with MS angle                                          ║
║ 112 ║ Line Fabrication Damage                                              ║
║ 113 ║ Heavy Wind                                                           ║
╚═════╩══════════════════════════════════════════════════════════════════════╝
*/

// ProcessedRow is one row in the result table returned by RunAutomation.
type ProcessedRow struct {
	OutageID   string  `json:"outage_id"`
	Hours      float64 `json:"hours"`
	Bucket     string  `json:"bucket"`
	Feeder     string  `json:"feeder"`
	ReasonID   int     `json:"reason_id"`
	ReasonName string  `json:"reason_name,omitempty"`
	Status     string  `json:"status"` // "submitted" | "skipped" | "failed" | "parse_error"
	Note       string  `json:"note,omitempty"`
}

var reasonNameMap = map[int]string{
	1:   "Pin Puncture",
	2:   "Pole collapse",
	4:   "Animal Fault",
	5:   "Bird Fault",
	6:   "Cable Wire Fault",
	7:   "Vehicle accident",
	8:   "Conductor Slipped From Pin Insulator",
	9:   "Conductor Snapped HT Line",
	10:  "Disaster like heavy rain",
	12:  "Flash over of Breakers",
	15:  "Guarding Fault",
	16:  "Hoarding Fallen",
	17:  "HT Connection Internal Fault",
	18:  "Insulation Burnt",
	19:  "Insulator Puncture",
	20:  "Jumper Burnt",
	21:  "Jumper Touching",
	22:  "LA Fault",
	23:  "Lightening Stroke",
	24:  "Low Clearance at Crossing",
	25:  "No Cause found",
	27:  "Overhead ABC conductor fault",
	28:  "Relay Problems",
	29:  "Shakle Puncture",
	30:  "Transformer Failure",
	31:  "Tree / Tree Branch Falling",
	32:  "Under ground cable fault",
	33:  "Under Ground Cable Fault by Outsider",
	44:  "General Maintenance",
	65:  "Smoke",
	74:  "Conductor Snapped HT Line",
	75:  "Accident",
	78:  "Jumper",
	86:  "Danger to life",
	113: "Heavy Wind",
}

func getReasonName(reasonID int, ruleName string) string {
	if ruleName != "" {
		return ruleName
	}
	if name, ok := reasonNameMap[reasonID]; ok {
		return name
	}
	return ""
}

// RunResult is what the HTTP /run endpoint returns and what the CLI prints.
type RunResult struct {
	EmpNo      string         `json:"emp_no,omitempty"`
	Total      int            `json:"total"`
	Success    int            `json:"success"`
	Failed     int            `json:"failed"`
	Skipped    int            `json:"skipped"`
	Rows       []ProcessedRow `json:"rows"`
	StartedAt  time.Time      `json:"started_at"`
	DurationMs int64          `json:"duration_ms"`
}

// RunAutomation executes the full pipeline once for a given user profile. All progress lines are
// written to `out`; the structured outcome is returned in RunResult.
func RunAutomation(profile models.UserProfile, limit int, enabledReasonIDs map[int]bool, out io.Writer) (*RunResult, error) {
	lg := log.New(out, "", log.LstdFlags)

	startedAt := time.Now()
	result := &RunResult{EmpNo: profile.EmpNo, StartedAt: startedAt}

	lg.Println("═══ OMS Outage Reason Automation ═══")
	lg.Printf("👤 Profile: EmpNo=%s (Company=%s)", profile.EmpNo, profile.CompanyName)
	if limit > 0 {
		lg.Printf("⚙ Limit: Processing max %d outages", limit)
	}

	client := oms.NewClient(profile)

	lg.Println("[Step 0] Logging in...")
	if err := client.Login(); err != nil {
		return result, fmt.Errorf("login failed: %w", err)
	}

	lg.Println("[Step 1] Fetching pending outages...")
	outages, err := client.FetchPendingOutages(limit)
	if err != nil {
		return result, fmt.Errorf("fetch pending: %w", err)
	}
	lg.Printf("  → Fetched %d outages", len(outages))

	type processedOutage struct {
		Outage        models.Outage
		DurationHours float64
		Rule          models.DurationRule
	}

	var processed []processedOutage
	for _, o := range outages {
		hours, err := utils.CalculateDurationFromTimestamps(
			o.OutageOccurDate, o.OutageOccurTime,
			o.OutageRestoreDate, o.OutageRestoreTime,
		)
		if err != nil {
			lg.Printf("  [WARN] Skip %s: %v", o.ID, err)
			result.Rows = append(result.Rows, ProcessedRow{
				OutageID: o.ID,
				Feeder:   o.FeederName,
				Status:   "parse_error",
				Note:     err.Error(),
			})
			continue
		}

		rule := utils.ClassifyRule(hours, config.DurationRules)

		// Special rule: KUMBHIYA feeder with duration > 6 hours → reason 25
		if o.FeederName == "KUMBHIYA" && hours > 6 {
			rule = models.DurationRule{Label: "KUMBHIYA >6h", MaxHours: 0, ReasonID: 25}
		}

		processed = append(processed, processedOutage{
			Outage:        o,
			DurationHours: hours,
			Rule:          rule,
		})
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "┌────────────────┬────────┬────────────────┬──────────────────┬─────────────────────────────────────────┐")
	fmt.Fprintln(out, "│ Outage ID      │ Hours  │ Bucket         │ Feeder           │ Reason                                  │")
	fmt.Fprintln(out, "├────────────────┼────────┼────────────────┼──────────────────┼─────────────────────────────────────────┤")
	for _, p := range processed {
		rName := getReasonName(p.Rule.ReasonID, p.Rule.ReasonName)
		rStr := fmt.Sprintf("#%d %s", p.Rule.ReasonID, rName)
		fmt.Fprintf(out, "│ %-14s │ %5.2f  │ %-14s │ %-16s │ %-39s │\n",
			p.Outage.ID, p.DurationHours,
			p.Rule.Label, p.Outage.FeederName, rStr)
	}
	fmt.Fprintln(out, "└────────────────┴────────┴────────────────┴──────────────────┴─────────────────────────────────────────┘")

	toProcess := processed
	if limit > 0 && len(processed) > limit {
		toProcess = processed[:limit]
		lg.Printf("⚙ Limiting to first %d outages (out of %d total)", limit, len(processed))
	}

	result.Total = len(toProcess)
	lg.Printf("[Step 2 & 3] Processing %d outages...", len(toProcess))

	for i, p := range toProcess {
		id := p.Outage.ID
		rName := getReasonName(p.Rule.ReasonID, p.Rule.ReasonName)
		row := ProcessedRow{
			OutageID:   id,
			Hours:      p.DurationHours,
			Bucket:     p.Rule.Label,
			Feeder:     p.Outage.FeederName,
			ReasonID:   p.Rule.ReasonID,
			ReasonName: rName,
		}

		isOverride := p.Rule.MaxHours == 0 && p.Rule.ReasonID != 0
		is1573 := p.DurationHours >= 15.72 && p.DurationHours <= 15.74
		if p.DurationHours > 8 && !is1573 && !isOverride {
			lg.Printf("  [%d/%d] Outage %s | %.2fh | ⊘ SKIPPED (no matching rule)",
				i+1, len(toProcess), id, p.DurationHours)
			row.Status = "skipped"
			row.Note = "no matching rule"
			result.Rows = append(result.Rows, row)
			result.Skipped++
			continue
		}

		if enabledReasonIDs != nil && !enabledReasonIDs[p.Rule.ReasonID] {
			lg.Printf("  [%d/%d] Outage %s | %.2fh | ⊘ SKIPPED (interval '%s' deselected)",
				i+1, len(toProcess), id, p.DurationHours, p.Rule.Label)
			row.Status = "skipped"
			row.Note = fmt.Sprintf("interval '%s' deselected", p.Rule.Label)
			result.Rows = append(result.Rows, row)
			result.Skipped++
			continue
		}

		lg.Printf("  [%d/%d] Outage %s | %.2fh | reason=#%d (%s)",
			i+1, len(toProcess), id, p.DurationHours, p.Rule.ReasonID, rName)

		locIDs, err := client.FetchLocIDs(id, p.Outage.FeederID)
		if err == oms.ErrNoGeoLocation {
			lg.Printf("    → No geo location found. Submitting general maintenance reason...")
			if err := client.SubmitNoGeoReason(id); err != nil {
				lg.Printf("    ✗ Submit failed: %v", err)
				row.Status = "failed"
				row.Note = "submit no_geo: " + err.Error()
				result.Rows = append(result.Rows, row)
				result.Failed++
				continue
			}
			lg.Printf("    ✓ Submitted (General Maintenance)")
			row.Status = "submitted"
			row.Note = "no geo location (General Maintenance)"
			row.ReasonID = 44
			result.Rows = append(result.Rows, row)
			result.Success++

			lg.Printf("    → Waiting %dms before next outage...", config.DelayBetweenOutages)
			time.Sleep(time.Duration(config.DelayBetweenOutages) * time.Millisecond)
			continue
		} else if err != nil {
			lg.Printf("    ✗ loc_ids fetch failed: %v", err)
			row.Status = "failed"
			row.Note = "loc_ids fetch: " + err.Error()
			result.Rows = append(result.Rows, row)
			result.Failed++
			continue
		}

		var pickedLocID int
		if p.DurationHours <= 0.25 {
			pickedLocID = locIDs[0]
			lg.Printf("    → loc_id=%d (1st pole selected for ≤15 min outage, from %d poles)", pickedLocID, len(locIDs))
		} else {
			pickedLocID = locIDs[rand.Intn(len(locIDs))]
			lg.Printf("    → loc_id=%d (randomly picked from %d poles)", pickedLocID, len(locIDs))
		}

		if err := client.SubmitReason(id, pickedLocID, p.Rule.ReasonID); err != nil {
			lg.Printf("    ✗ Submit failed: %v", err)
			row.Status = "failed"
			row.Note = "submit: " + err.Error()
			result.Rows = append(result.Rows, row)
			result.Failed++
			continue
		}

		lg.Printf("    ✓ Submitted")
		row.Status = "submitted"
		result.Rows = append(result.Rows, row)
		result.Success++

		lg.Printf("    → Waiting %dms before next outage...", config.DelayBetweenOutages)
		time.Sleep(time.Duration(config.DelayBetweenOutages) * time.Millisecond)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "─── Results ───")
	fmt.Fprintf(out, "  Total:   %d\n", result.Total)
	fmt.Fprintf(out, "  Success: %d\n", result.Success)
	fmt.Fprintf(out, "  Failed:  %d\n", result.Failed)
	fmt.Fprintf(out, "  Skipped: %d\n", result.Skipped)
	lg.Println("═══ Done ═══")

	result.DurationMs = time.Since(startedAt).Milliseconds()
	return result, nil
}

func main() {
	// Force IST for all time operations regardless of host TZ.
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		log.Fatal("❌ Failed to load IST timezone:", err)
	}
	time.Local = ist
	rand.Seed(time.Now().UnixNano())

	serverFlag := flag.Bool("server", false, "Run as HTTP server instead of one-shot CLI")
	limitFlag := flag.Int("limit", 0, "Limit number of outages to process (0 = process all)")
	profileFlag := flag.Int("profile", 1, "Profile index (1-based) to run in CLI mode")
	rulesFlag := flag.String("rules", "", "Comma-separated allowed reason IDs (e.g. 21,20,31,74,28)")
	flag.Parse()

	if *serverFlag || os.Getenv("RUN_MODE") == "server" {
		runServer()
		return
	}

	// Positional argument support: ./oms-automtion 22
	if len(flag.Args()) > 0 {
		if val, err := strconv.Atoi(flag.Args()[0]); err == nil {
			*limitFlag = val
		}
	}

	profiles := config.LoadProfiles()
	targetProfile := profiles[0]
	if *profileFlag > 0 && *profileFlag <= len(profiles) {
		targetProfile = profiles[*profileFlag-1]
	}

	var enabledReasonIDs map[int]bool
	if *rulesFlag != "" {
		enabledReasonIDs = make(map[int]bool)
		for _, part := range strings.Split(*rulesFlag, ",") {
			part = strings.TrimSpace(part)
			if id, err := strconv.Atoi(part); err == nil {
				enabledReasonIDs[id] = true
			}
		}
	}

	if _, err := RunAutomation(targetProfile, *limitFlag, enabledReasonIDs, os.Stdout); err != nil {
		log.Fatalf("FATAL: %v", err)
	}
}
