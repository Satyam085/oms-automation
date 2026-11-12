package main

import (
	"errors"
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
OMS reason IDs, as listed in the OMS web UI. The duration rules submit from the
first list; the no-geo path (ESD/PSD, no pole to pick) submits #44.

Sustained fault (a pole location is selected):
   1 Pin Puncture                        25 No Cause found
   2 Pole collapse                       27 Overhead ABC conductor fault
   4 Animal Fault                        28 Relay Problems
   5 Bird Fault                          29 Shakle Puncture
   6 Cable Wire Fault                    30 Transformer Failure
   7 Vehicle accident                    31 Tree / Tree Branch Falling
   8 Conductor Slipped From Pin Insul.   32 Under ground cable fault
   9 Conductor Snapped HT Line           33 Under Ground Cable Fault by Outsider
  10 Disaster like heavy rain            65 Smoke
  12 Flash over of Breakers              74 Cyclone
  15 Guarding Fault                      75 Accident
  16 Hoarding Fallen                     78 Jumper
  17 HT Connection Internal Fault        86 Danger to life
  18 Insulation Burnt                    87 Bomb blast
  19 Insulator Puncture                  88 Air Strike
  20 Jumper Burnt                        91 Fire in buildings
  21 Jumper Touching                     92 Fire in godown
  22 LA Fault                            93 Fire in fiels/jungle
  23 Lightening Stroke                  100 DO Fuse short with MS angle
  24 Low Clearance at Crossing          112 Line Fabrication Damage
                                        113 Heavy Wind

Emergency / planned shutdown (ESD/PSD, no location):
   1 Pin Puncture                        55 Pole Erection
   2 Pole collapse                       56 Replacement of Transformer
   9 Conductor Snapped HT Line           57 Saftey Purpose
  20 Jumper Burnt                        58 SS Shutdown
  24 Low Clearance at Crossing           60 Thread Burning
  38 Capital Work                        61 To Attend Fault
  39 Conductor Jointing                  62 Transformer Dismentaling
  40 Cross Arm Replacement               63 Transformer Replacement
  41 Disc Puncture                       64 Tree Cutting
  42 Erection of Transformer             65 Smoke
  43 Feeder Charging                     74 Cyclone
  44 General Maintenance                 75 Accident
  45 Guarding Removal                    76 Mob Attack
  46 Insulator Replacement               82 Could not reach
  47 Jumper Replacement                  83 Extension of work
  48 Kite Thread Removal                 84 Flood
  49 Line Dismentaling                   86 Danger to life
  50 Line Erection                       87 Bomb blast
  51 Load Change                         88 Air Strike
  53 Natural Disaster                    95 Pole tilt
  54 Pin/Shackle Replacement             96 Due to SDT
                                         98 Conductor Snapped LT Line
                                         99 Switch Operation
                                        100 DO Fuse short with MS angle
                                        101 Load Shedding
                                        112 Line Fabrication Damage
                                        113 Heavy Wind
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

// relayFeeders trip for roughly 15 hours at a time; an outage of 14.5–16.5h on
// one of them is #28 Relay Problems. Names are OMS feeder_name values.
var relayFeeders = map[string]bool{"KUMBHIYA": true, "KHAKHAR": true}

// ruleFor picks the duration rule for an outage: the feeder overrides first, then
// the duration buckets (config.DurationRules, all ≤ 8h). Anything else over 8
// hours has no real bucket and gets #25 No Cause found instead of being skipped.
func ruleFor(hours float64, feederName string) models.DurationRule {
	if relayFeeders[feederName] && hours >= 14.5 && hours <= 16.5 {
		return models.DurationRule{Label: "14.5–16.5h", ReasonID: 28, ReasonName: "Relay Problems"}
	}

	noCause := models.DurationRule{Label: "> 8 hours", ReasonID: 25, ReasonName: "No Cause found"}
	if feederName == "KUMBHIYA" && hours > 6 {
		noCause.Label = "KUMBHIYA >6h"
		return noCause
	}
	if hours > 8 {
		return noCause
	}
	return utils.ClassifyRule(hours, config.DurationRules)
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

// feederPoles caches HT pole IDs per feeder for one run. The detail GET returns
// the whole feeder's poles — 1 MB+ and ~2s for a big feeder — and they are the
// same for every outage on it, so a run fetches them once per feeder instead of
// once per outage. Only sustained faults share the cache: shutdowns (ESD/PSD)
// come back with no poles and must take the no-geo path, and their GET is tiny.
type feederPoles map[int][]int

// locIDs returns the outage's candidate poles and whether they came from the
// cache. fetch is the detail GET (oms.Client.FetchLocIDs).
func (fp feederPoles) locIDs(o models.Outage, fetch func(outageID string, feederID int) ([]int, error)) ([]int, bool, error) {
	fault := o.OutageTypeName == "Sustain Fault"
	if ids, ok := fp[o.FeederID]; ok && fault {
		return ids, true, nil
	}
	ids, err := fetch(o.ID, o.FeederID)
	if err == nil && fault {
		fp[o.FeederID] = ids
	}
	return ids, false, err
}

// freshOutages returns the outages not already seen, marking them seen. A refetch
// re-serves anything we could not clear, and reprocessing those would double-count.
func freshOutages(seen map[string]bool, outages []models.Outage) []models.Outage {
	var fresh []models.Outage
	for _, o := range outages {
		if !seen[o.ID] {
			seen[o.ID] = true
			fresh = append(fresh, o)
		}
	}
	return fresh
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

	client := oms.NewClient(profile, lg)
	pageSize := config.PageSize()
	lg.Printf("⚙ Pacing: ≥%s between OMS requests, %d outages per page", client.Gap, pageSize)

	lg.Println("[Step 0] Logging in...")
	if err := client.Login(); err != nil {
		return result, fmt.Errorf("login failed: %w", err)
	}

	type processedOutage struct {
		Outage        models.Outage
		DurationHours float64
		Rule          models.DurationRule
	}

	// Page at a time: pending is a queue. Clearing a page drops those outages off the
	// list and pulls fresh ones up into its place, so we refetch instead of paginating
	// through a list that shifts under us — and only one slow /pending call is in flight.
	// `stuck` is what we saw but could not clear (parse errors, deselected intervals,
	// submit failures); those stay at the head of the queue, so the next fetch starts
	// past them. A page with nothing new on it means there is nothing left to do.
	seen := map[string]bool{}
	poles := feederPoles{}
	stuck := 0
	attempted := 0

	for page := 1; ; page++ {
		lg.Printf("[Step 1] Fetching pending page %d (offset=%d)...", page, stuck)
		outages, totalPending, err := client.FetchPendingPage(stuck, pageSize)
		if err != nil {
			return result, fmt.Errorf("fetch pending: %w", err)
		}

		fresh := freshOutages(seen, outages)
		lg.Printf("  → Page has %d outages, %d new (%d pending overall)", len(outages), len(fresh), totalPending)
		if len(fresh) == 0 {
			lg.Println("  → Nothing new left to clear")
			break
		}

		var processed []processedOutage
		for _, o := range fresh {
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
				result.Total++
				result.Failed++
				stuck++
				continue
			}

			processed = append(processed, processedOutage{
				Outage:        o,
				DurationHours: hours,
				Rule:          ruleFor(hours, o.FeederName),
			})
		}

		toProcess := processed
		if limit > 0 && attempted+len(toProcess) > limit {
			toProcess = toProcess[:limit-attempted]
			lg.Printf("⚙ Limiting to %d outages on this page (limit %d)", len(toProcess), limit)
		}

		fmt.Fprintln(out)
		fmt.Fprintln(out, "┌────────────────┬────────┬────────────────┬──────────────────┬─────────────────────────────────────────┐")
		fmt.Fprintln(out, "│ Outage ID      │ Hours  │ Bucket         │ Feeder           │ Reason                                  │")
		fmt.Fprintln(out, "├────────────────┼────────┼────────────────┼──────────────────┼─────────────────────────────────────────┤")
		for _, p := range toProcess {
			rStr := fmt.Sprintf("#%d %s", p.Rule.ReasonID, p.Rule.ReasonName)
			fmt.Fprintf(out, "│ %-14s │ %5.2f  │ %-14s │ %-16s │ %-39s │\n",
				p.Outage.ID, p.DurationHours,
				p.Rule.Label, p.Outage.FeederName, rStr)
		}
		fmt.Fprintln(out, "└────────────────┴────────┴────────────────┴──────────────────┴─────────────────────────────────────────┘")

		attempted += len(toProcess)
		lg.Printf("[Step 2 & 3] Processing %d outages...", len(toProcess))

		for i, p := range toProcess {
			result.Total++
			id := p.Outage.ID
			rName := p.Rule.ReasonName
			row := ProcessedRow{
				OutageID:   id,
				Hours:      p.DurationHours,
				Bucket:     p.Rule.Label,
				Feeder:     p.Outage.FeederName,
				ReasonID:   p.Rule.ReasonID,
				ReasonName: rName,
			}

			if enabledReasonIDs != nil && !enabledReasonIDs[p.Rule.ReasonID] {
				lg.Printf("  [%d/%d] Outage %s | %.2fh | ⊘ SKIPPED (interval '%s' deselected)",
					i+1, len(toProcess), id, p.DurationHours, p.Rule.Label)
				row.Status = "skipped"
				row.Note = fmt.Sprintf("interval '%s' deselected", p.Rule.Label)
				result.Rows = append(result.Rows, row)
				result.Skipped++
				stuck++
				continue
			}

			lg.Printf("  [%d/%d] Outage %s | %.2fh | reason=#%d (%s)",
				i+1, len(toProcess), id, p.DurationHours, p.Rule.ReasonID, rName)

			locIDs, cached, err := poles.locIDs(p.Outage, client.FetchLocIDs)
			from := fmt.Sprintf("%d poles", len(locIDs))
			if cached {
				from += ", cached for this feeder"
			}
			if err == oms.ErrNoGeoLocation {
				lg.Printf("    → No geo location found. Submitting general maintenance reason...")
				if err := client.SubmitNoGeoReason(id); err != nil {
					lg.Printf("    ✗ Submit failed: %v", err)
					row.Status = "failed"
					row.Note = "submit no_geo: " + err.Error()
					result.Rows = append(result.Rows, row)
					result.Failed++
					stuck++
					if errors.Is(err, oms.ErrRateLimited) {
						return result, err
					}
					continue
				}
				lg.Printf("    ✓ Submitted (General Maintenance)")
				row.Status = "submitted"
				row.Note = "no geo location (General Maintenance)"
				row.ReasonID = 44
				row.ReasonName = "General Maintenance"
				result.Rows = append(result.Rows, row)
				result.Success++
				continue
			} else if err != nil {
				lg.Printf("    ✗ loc_ids fetch failed: %v", err)
				row.Status = "failed"
				row.Note = "loc_ids fetch: " + err.Error()
				result.Rows = append(result.Rows, row)
				result.Failed++
				stuck++
				if errors.Is(err, oms.ErrRateLimited) {
					return result, err
				}
				continue
			}

			var pickedLocID int
			if p.DurationHours <= 0.25 {
				pickedLocID = locIDs[0]
				lg.Printf("    → loc_id=%d (1st pole selected for ≤15 min outage, from %s)", pickedLocID, from)
			} else {
				pickedLocID = locIDs[rand.Intn(len(locIDs))]
				lg.Printf("    → loc_id=%d (randomly picked from %s)", pickedLocID, from)
			}

			if err := client.SubmitReason(id, pickedLocID, p.Rule.ReasonID); err != nil {
				lg.Printf("    ✗ Submit failed: %v", err)
				row.Status = "failed"
				row.Note = "submit: " + err.Error()
				result.Rows = append(result.Rows, row)
				result.Failed++
				stuck++
				if errors.Is(err, oms.ErrRateLimited) {
					return result, err
				}
				continue
			}

			lg.Printf("    ✓ Submitted")
			row.Status = "submitted"
			result.Rows = append(result.Rows, row)
			result.Success++
		}

		if limit > 0 && attempted >= limit {
			lg.Printf("⚙ Limit of %d reached", limit)
			break
		}
		if stuck >= totalPending {
			lg.Println("  → Nothing left but outages we could not clear")
			break
		}
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

	serverFlag := flag.Bool("server", false, "Run as HTTP server instead of one-shot CLI")
	limitFlag := flag.Int("limit", 0, "Limit number of outages to process (0 = process all)")
	profileFlag := flag.Int("profile", 1, "Profile index (1-based) to run in CLI mode")
	rulesFlag := flag.String("rules", "", "Comma-separated allowed reason IDs (e.g. 21,20,31,9,28)")
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
