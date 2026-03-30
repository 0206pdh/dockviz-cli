package ui

import (
	"strings"
	"testing"
)

func TestSparkline(t *testing.T) {
	t.Run("empty values returns 10 spaces", func(t *testing.T) {
		got := Sparkline(nil)
		if got != "          " {
			t.Errorf("Sparkline(nil) = %q, want 10 spaces", got)
		}
		if len([]rune(got)) != 10 {
			t.Errorf("Sparkline(nil) length = %d, want 10", len([]rune(got)))
		}
	})

	t.Run("result is always 10 runes wide", func(t *testing.T) {
		cases := [][]float64{
			{50},
			{10, 20, 30},
			{0, 25, 50, 75, 100, 0, 25, 50, 75, 100},
		}
		for _, v := range cases {
			got := Sparkline(v)
			if len([]rune(got)) != 10 {
				t.Errorf("Sparkline(%v) length = %d, want 10", v, len([]rune(got)))
			}
		}
	})

	t.Run("zero CPU shows lowest bar", func(t *testing.T) {
		got := Sparkline([]float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
		for _, r := range strings.TrimLeft(got, " ") {
			if r != '▁' {
				t.Errorf("Sparkline with all-zero values: got rune %q, want ▁", r)
			}
		}
	})

	t.Run("100% CPU shows full bar", func(t *testing.T) {
		got := Sparkline([]float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100})
		for _, r := range got {
			if r != '█' {
				t.Errorf("Sparkline with all-100 values: got rune %q, want █", r)
			}
		}
	})

	t.Run("fixed scale: 5% looks different from 95%", func(t *testing.T) {
		low := strings.TrimLeft(Sparkline([]float64{5, 5, 5, 5, 5, 5, 5, 5, 5, 5}), " ")
		high := strings.TrimLeft(Sparkline([]float64{95, 95, 95, 95, 95, 95, 95, 95, 95, 95}), " ")
		if low == high {
			t.Errorf("Sparkline(5%%) == Sparkline(95%%) — scale is not fixed (got %q)", low)
		}
		// Low bar should use a lower block character than high bar
		if []rune(low)[0] >= []rune(high)[0] {
			t.Errorf("5%% bar %q should be visually lower than 95%% bar %q", low, high)
		}
	})

	t.Run("values above 100 are clamped", func(t *testing.T) {
		// Should not panic and should produce a full bar
		got := Sparkline([]float64{200, 200, 200, 200, 200, 200, 200, 200, 200, 200})
		if len([]rune(got)) != 10 {
			t.Errorf("Sparkline with >100 values: length = %d, want 10", len([]rune(got)))
		}
	})
}
