package tui

import (
	"testing"
	"time"

	"github.com/0206pdh/dockviz-cli/internal/docker"
)

func TestProblemsDetectsAndResolvesAbnormalExit(t *testing.T) {
	now := time.Now()
	m := Model{events: []docker.EventInfo{
		{Time: now, Action: "die", ContainerName: "api", ExitCode: 137, OOMKilled: true},
	}}
	problems := m.problems()
	if len(problems) != 1 || problems[0].Kind != "OOM killed" {
		t.Fatalf("problems() = %+v, want one OOM problem", problems)
	}

	m.events = append([]docker.EventInfo{{Time: now.Add(time.Second), Action: "start", ContainerName: "api"}}, m.events...)
	if got := m.problems(); len(got) != 0 {
		t.Fatalf("problems() after start = %+v, want no active problems", got)
	}
}

func TestProblemsDetectsRestartLoop(t *testing.T) {
	now := time.Now()
	m := Model{events: []docker.EventInfo{
		{Time: now, Action: "restart", ContainerName: "worker"},
		{Time: now.Add(-time.Minute), Action: "restart", ContainerName: "worker"},
		{Time: now.Add(-2 * time.Minute), Action: "restart", ContainerName: "worker"},
	}}
	problems := m.problems()
	if len(problems) != 1 || problems[0].Kind != "Restart loop" {
		t.Fatalf("problems() = %+v, want one restart-loop problem", problems)
	}
}

func TestProblemsReportsDisconnectedDaemon(t *testing.T) {
	m := Model{eventDisconnected: true}
	problems := m.problems()
	if len(problems) != 1 || problems[0].Kind != "Daemon disconnected" {
		t.Fatalf("problems() = %+v, want daemon-disconnected problem", problems)
	}
}

func TestProblemsDetectResourceIssues(t *testing.T) {
	m := Model{
		containers: []docker.ContainerInfo{
			{
				ID:          "cpu",
				Name:        "cpu-hog",
				Status:      "running",
				LimitsKnown: true,
				CPULimit:    1,
			},
			{
				ID:            "mem",
				Name:          "mem-pressure",
				Status:        "running",
				LimitsKnown:   true,
				MemoryLimitMB: 100,
			},
			{
				ID:          "nolimit",
				Name:        "nolimit-api",
				Status:      "running",
				LimitsKnown: true,
			},
		},
		history: map[string][]float64{
			"cpu": []float64{90, 95, 91, 93},
		},
		memHistory: map[string][]float64{
			"mem": []float64{40, 55, 70, 85, 105, 120},
		},
	}

	problems := m.problems()
	wantKinds := map[string]bool{
		"High CPU":           false,
		"Memory pressure":    false,
		"Memory growth":      false,
		"No resource limits": false,
	}
	for _, p := range problems {
		if _, ok := wantKinds[p.Kind]; ok {
			wantKinds[p.Kind] = true
		}
	}
	for kind, seen := range wantKinds {
		if !seen {
			t.Fatalf("missing problem kind %q in %+v", kind, problems)
		}
	}
}
