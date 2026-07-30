package tui

import (
	"testing"

	"github.com/0206pdh/dockviz-cli/internal/docker"
)

func TestSummarizeResource(t *testing.T) {
	s := summarizeResource([]float64{1, 2, 3, 4, 100})
	if s.Current != 100 {
		t.Fatalf("Current = %v, want 100", s.Current)
	}
	if s.Avg != 22 {
		t.Fatalf("Avg = %v, want 22", s.Avg)
	}
	if s.P95 != 100 {
		t.Fatalf("P95 = %v, want 100", s.P95)
	}
	if s.Peak != 100 {
		t.Fatalf("Peak = %v, want 100", s.Peak)
	}
	if s.Trend != "up" {
		t.Fatalf("Trend = %q, want up", s.Trend)
	}
}

func TestResourceTrend(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   string
	}{
		{"short is flat", []float64{1, 2}, "flat"},
		{"up", []float64{1, 1, 1, 5, 5, 5}, "up"},
		{"down", []float64{5, 5, 5, 1, 1, 1}, "down"},
		{"flat", []float64{10, 10.5, 10, 10.4, 10.2, 10.1}, "flat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resourceTrend(tt.values); got != tt.want {
				t.Fatalf("resourceTrend() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatLimits(t *testing.T) {
	tests := []struct {
		name string
		c    docker.ContainerInfo
		want string
	}{
		{
			name: "unknown",
			c:    docker.ContainerInfo{},
			want: "unknown",
		},
		{
			name: "no limits",
			c:    docker.ContainerInfo{LimitsKnown: true},
			want: "CPU:- MEM:-",
		},
		{
			name: "cpu and memory limits",
			c: docker.ContainerInfo{
				LimitsKnown:   true,
				CPULimit:      1.5,
				MemoryLimitMB: 2048,
			},
			want: "CPU:1.5 MEM:2.0GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatLimits(tt.c); got != tt.want {
				t.Fatalf("formatLimits() = %q, want %q", got, tt.want)
			}
		})
	}
}
