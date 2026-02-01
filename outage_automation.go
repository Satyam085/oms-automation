package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"oms-automtion/config"
	"oms-automtion/models"
	"oms-automtion/oms"
	"oms-automtion/utils"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	log.Println("═══ OMS Outage Reason Automation ═══")

	client := oms.NewClient()

	// ── Step 0: Login & get token ──
	log.Println("\n[Step 0] Logging in...")
	if err := client.Login(); err != nil {
		log.Fatalf("FATAL: login failed: %v", err)
	}

	// ── Step 1 ──
	log.Println("\n[Step 1] Fetching pending outages...")
	outages, err := client.FetchPendingOutages()
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
		hours, err := utils.ParseDuration(o.Duration)
		if err != nil {
			log.Printf("  [WARN] Skip %s: %v", o.ID, err)
			continue
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

	// ── Steps 2 & 3: fetch loc_id → submit reason ──
	log.Printf("\n[Step 2 & 3] Processing %d outages...\n", len(processed))

	successCount := 0
	failCount := 0

	for i, p := range processed {
		id := p.Outage.ID
		log.Printf("  [%d/%d] Outage %s | %.2fh | reason_id=%d",
			i+1, len(processed), id, p.DurationHours, p.Rule.ReasonID)

		// Fetch available loc_ids from feederPointGeoJson
		locIDs, err := client.FetchLocIDs(id)
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

		// // Submit (Commented out in original)
		// if err := client.SubmitReason(id, pickedLocID, p.Rule.ReasonID); err != nil {
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