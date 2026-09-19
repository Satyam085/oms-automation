package main

import (
	"testing"

	"oms-automtion/models"
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
		{7, "X", 74},
		{9, "X", 25}, // was skipped as "no matching rule"
		{15.73, "X", 28},
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
