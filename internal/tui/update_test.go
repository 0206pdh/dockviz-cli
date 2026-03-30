package tui

import "testing"

func TestClamp(t *testing.T) {
	tests := []struct {
		name string
		v    int
		lo   int
		hi   int
		want int
	}{
		{"within range", 5, 0, 10, 5},
		{"below lo returns lo", -1, 0, 10, 0},
		{"above hi returns hi", 15, 0, 10, 10},
		{"equal lo", 0, 0, 10, 0},
		{"equal hi", 10, 0, 10, 10},
		{"hi less than lo returns lo", 5, 5, 3, 5},
		{"zero range", 0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clamp(tt.v, tt.lo, tt.hi)
			if got != tt.want {
				t.Errorf("clamp(%d, %d, %d) = %d, want %d", tt.v, tt.lo, tt.hi, got, tt.want)
			}
		})
	}
}
