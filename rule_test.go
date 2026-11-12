package main

import (
	"slices"
	"testing"

	"oms-automtion/models"
	"oms-automtion/oms"
)

func TestRuleFor(t *testing.T) {
	cases := []struct {
		hours  float64
		feeder string
		want   int
	}{
		{0.1, "X", 21},
		{0.5, "X", 20},
		{2, "X", 31},
		{7, "X", 9},
		{9, "X", 25},     // was skipped as "no matching rule"
		{15.73, "X", 25}, // the old exact-15.73h window is gone for other feeders
		{7, "KHAKHAR", 9},
		// ~15h on the relay feeders is #28, inclusive at both ends
		{14.5, "KUMBHIYA", 28},
		{15.02, "KUMBHIYA", 28},
		{16.04, "KUMBHIYA", 28},
		{16.5, "KUMBHIYA", 28},
		{14.51, "KHAKHAR", 28},
		{16.5, "KHAKHAR", 28},
		// just outside it, the usual over-range rules apply
		{14.49, "KUMBHIYA", 25},
		{16.51, "KUMBHIYA", 25},
		{14.49, "KHAKHAR", 25},
		{16.51, "KHAKHAR", 25},
		{40, "X", 25},
		{7, "KUMBHIYA", 25},
	}
	for _, c := range cases {
		if got := ruleFor(c.hours, c.feeder); got.ReasonID != c.want {
			t.Errorf("ruleFor(%v, %q) = #%d, want #%d", c.hours, c.feeder, got.ReasonID, c.want)
		}
	}
}

func TestFreshOutagesSkipsAlreadySeen(t *testing.T) {
	seen := map[string]bool{}
	page1 := []models.Outage{{ID: "a"}, {ID: "b"}}
	if got := freshOutages(seen, page1); len(got) != 2 {
		t.Fatalf("first page: want 2 fresh, got %d", len(got))
	}
	// "b" was not cleared, so the refetch serves it again alongside a new record.
	page2 := []models.Outage{{ID: "b"}, {ID: "c"}}
	got := freshOutages(seen, page2)
	if len(got) != 1 || got[0].ID != "c" {
		t.Fatalf("refetch: want only [c], got %v", got)
	}
	if len(freshOutages(seen, page2)) != 0 {
		t.Fatal("nothing new should mean an empty slice (loop stop condition)")
	}
}

func TestFeederPolesFetchesOncePerFeederForFaults(t *testing.T) {
	var gets []string
	fetch := func(id string, feederID int) ([]int, error) {
		gets = append(gets, id)
		if id == "esd" {
			return nil, oms.ErrNoGeoLocation // shutdowns come back without poles
		}
		return []int{feederID * 10, feederID*10 + 1}, nil
	}
	fp := feederPoles{}
	fault := func(id string, feeder int) models.Outage {
		return models.Outage{ID: id, FeederID: feeder, OutageTypeName: "Sustain Fault"}
	}

	fp.locIDs(fault("a", 1), fetch)
	ids, cached, err := fp.locIDs(fault("b", 1), fetch)
	if err != nil || !cached || len(ids) != 2 || ids[0] != 10 {
		t.Fatalf("second fault on feeder 1: got %v cached=%v err=%v, want feeder 1's poles from cache", ids, cached, err)
	}
	fp.locIDs(fault("c", 2), fetch) // new feeder: must fetch

	// A shutdown on a feeder whose poles are cached must still be fetched, so it
	// takes the no-geo (#44) path instead of being submitted against a pole.
	esd := models.Outage{ID: "esd", FeederID: 1, OutageTypeName: "Emergency Shutdown"}
	if _, cached, err := fp.locIDs(esd, fetch); cached || err != oms.ErrNoGeoLocation {
		t.Fatalf("shutdown: cached=%v err=%v, want a fresh fetch returning ErrNoGeoLocation", cached, err)
	}
	if want := []string{"a", "c", "esd"}; !slices.Equal(gets, want) {
		t.Errorf("detail GETs %v, want %v", gets, want)
	}
}
