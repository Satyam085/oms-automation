package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strconv"
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
║ 29  ║ Shakle Puncture                                                      ║
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

Common Reason IDs Used by Duration Rules:
  - ID 25: No Cause found (< 1 hour)
  - ID 28: Relay Problems (1-4 hours, 4-8 hours)
  - ID 30: Transformer Failure (8-24 hours, > 24 hours)
*/


func main() {
	// Force Indian Standard Time (IST) for all time operations
	// This ensures consistent timestamps regardless of server timezone
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		log.Fatal("❌ Failed to load IST timezone:", err)
	}
	time.Local = ist

	// Parse command-line flags
	limitFlag := flag.Int("limit", 0, "Limit number of outages to process (0 = process all)")
	flag.Parse()
	
	// Support positional argument for limit (e.g. ./oms-automtion 22)
	if len(flag.Args()) > 0 {
		if val, err := strconv.Atoi(flag.Args()[0]); err == nil {
			*limitFlag = val
		}
	}

	rand.Seed(time.Now().UnixNano())
	log.Println("═══ OMS Outage Reason Automation ═══")
	
	if *limitFlag > 0 {
		log.Printf("⚙ Limit: Processing max %d outages\n", *limitFlag)
	}

	client := oms.NewClient()

	// ── Step 0: Login & get token ──
	log.Println("\n[Step 0] Logging in...")
	if err := client.Login(); err != nil {
		log.Fatalf("FATAL: login failed: %v", err)
	}

	// ── Step 1 ──
	log.Println("\n[Step 1] Fetching pending outages...")
	// Pass the limit to fetch only what's needed
	outages, err := client.FetchPendingOutages(*limitFlag)
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}
	log.Printf("  → Fetched %d outages\n", len(outages))

	// ── Parse & classify durations ──
	type processedOutage struct {
		Outage        models.Outage
		DurationHours float64
		Rule          models.DurationRule
	}

	var processed []processedOutage
	for _, o := range outages {
		// DEBUG: Print first 3 outages in detail
		// if i < 3 {
		// 	log.Printf("\n[DEBUG] Outage #%d:\n  ID: %s\n  Duration: %q\n  OutageOccurAt: %q\n  OutageRestoreAt: %q\n  FeederName: %q\n",
		// 		i+1, o.ID, o.Duration, o.OutageOccurAt, o.OutageRestoreAt, o.FeederName)
		// }

		// Try to calculate duration from timestamps first
		hours, err := utils.CalculateDurationFromTimestamps(o.OutageOccurAt, o.OutageRestoreAt)
		if err != nil {
			// Fallback: try parsing the pre-calculated duration field
			hours, err = utils.ParseDuration(o.Duration)
			if err != nil {
				log.Printf("  [WARN] Skip %s: %v", o.ID, err)
				continue
			}
		}
		
		processed = append(processed, processedOutage{
			Outage:        o,
			DurationHours: hours,
			Rule:          utils.ClassifyRule(hours, config.DurationRules),
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

	// Apply limit if specified
	toProcess := processed
	if *limitFlag > 0 && len(processed) > *limitFlag {
		toProcess = processed[:*limitFlag]
		log.Printf("\n⚙ Limiting to first %d outages (out of %d total)\n", *limitFlag, len(processed))
	}

	// ── Steps 2 & 3: fetch loc_id → submit reason ──
	log.Printf("\n[Step 2 & 3] Processing %d outages...\n", len(toProcess))

	successCount := 0
	failCount := 0

	for i, p := range toProcess {
		id := p.Outage.ID
		
		// Skip outages that don't match any rule (duration > 8 hours and != 15.73)
		// Use tolerance for floating-point comparison (within 0.01 hours = ~36 seconds)
		is1573 := p.DurationHours >= 15.72 && p.DurationHours <= 15.74
		if p.DurationHours > 8 && !is1573 {
			log.Printf("  [%d/%d] Outage %s | %.2fh | ⊘ SKIPPED (no matching rule)",
				i+1, len(toProcess), id, p.DurationHours)
			continue
		}
		
		log.Printf("  [%d/%d] Outage %s | %.2fh | reason_id=%d",
			i+1, len(toProcess), id, p.DurationHours, p.Rule.ReasonID)

		// Fetch available loc_ids from feederPointGeoJson
		locIDs, err := client.FetchLocIDs(id, p.Outage.FeederID)
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

		// Submit
		if err := client.SubmitReason(id, pickedLocID, p.Rule.ReasonID); err != nil {
			log.Printf("    ✗ Submit failed: %v", err)
			failCount++
			continue
		}

		log.Printf("    ✓ Submitted")
		successCount++

		// Rate limiting: delay between processing each outage
		log.Printf("    → Waiting %dms before next outage...", config.DelayBetweenOutages)
		time.Sleep(time.Duration(config.DelayBetweenOutages) * time.Millisecond)
	}

	// ── Summary ──
	fmt.Println()
	fmt.Println("─── Results ───")
	fmt.Printf("  Total:   %d\n", len(processed))
	fmt.Printf("  Success: %d\n", successCount)
	fmt.Printf("  Failed:  %d\n", failCount)
	log.Println("\n═══ Done ═══")
}