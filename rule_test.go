package main

import "testing"

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
