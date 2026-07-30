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
	if len(problems) != 1 || problems[0].Kind != "Restart loop" || problems[0].Severity != severityCritical {
		t.Fatalf("problems() = %+v, want one critical restart-loop problem", problems)
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
		"Memory over limit":  false,
		"Memory growth":      false,
		"No resource limits": false,
	}
	for _, p := range problems {
		if _, ok := wantKinds[p.Kind]; ok {
			wantKinds[p.Kind] = true
			if p.Recommendation == "" {
				t.Fatalf("problem %q has empty recommendation: %+v", p.Kind, p)
			}
		}
	}
	for kind, seen := range wantKinds {
		if !seen {
			t.Fatalf("missing problem kind %q in %+v", kind, problems)
		}
	}
}

func TestProblemsClassifySeverityLevels(t *testing.T) {
	m := Model{
		containers: []docker.ContainerInfo{
			{
				ID:            "critical-mem",
				Name:          "critical-mem",
				Status:        "running",
				LimitsKnown:   true,
				MemoryLimitMB: 100,
			},
			{
				ID:          "warning-cpu",
				Name:        "warning-cpu",
				Status:      "running",
				LimitsKnown: true,
				CPULimit:    1,
			},
			{
				ID:          "info-cpu",
				Name:        "info-cpu",
				Status:      "running",
				LimitsKnown: true,
				CPULimit:    1,
			},
			{
				ID:            "idle",
				Name:          "idle-db",
				Status:        "running",
				LimitsKnown:   true,
				MemoryLimitMB: 2048,
			},
		},
		history: map[string][]float64{
			"warning-cpu": []float64{80, 85, 88},
			"info-cpu":    []float64{60, 65, 62},
			"idle":        []float64{1, 2, 3},
		},
		memHistory: map[string][]float64{
			"critical-mem": []float64{90, 100, 110},
			"idle":         []float64{700, 720, 710},
		},
	}

	problems := m.problems()
	want := map[string]string{
		"Memory over limit": severityCritical,
		"High CPU":          severityWarning,
		"Elevated CPU":      severityInfo,
		"Idle memory":       severityInfo,
	}
	for _, p := range problems {
		if sev, ok := want[p.Kind]; ok && p.Severity != sev {
			t.Fatalf("%s severity = %s, want %s in %+v", p.Kind, p.Severity, sev, problems)
		}
	}
	for kind := range want {
		if !hasProblemKind(problems, kind) {
			t.Fatalf("missing %q in %+v", kind, problems)
		}
	}
}

func TestProblemsDetectProjectAndStorageOffenders(t *testing.T) {
	m := Model{
		diskUsageLoaded: true,
		diskUsage: docker.DiskUsageInfo{
			Images:     docker.DiskUsageCategory{ReclaimMB: 1200, OutsidePruneMB: 1400},
			Containers: docker.DiskUsageCategory{ReclaimMB: 2200},
			Volumes:    docker.DiskUsageCategory{ReclaimMB: 1500},
			BuildCache: docker.DiskUsageCategory{ReclaimMB: 3000},
			Logs:       docker.DiskUsageCategory{ReclaimMB: 2500},
			HostStorage: docker.HostStorageInfo{
				Label:       "Docker Desktop VHDX",
				AllocatedMB: 20000,
				HostFreeMB:  12000,
				Available:   true,
			},
		},
		containers: []docker.ContainerInfo{
			{Name: "worker", Status: "running", CPUPerc: 90, ComposeProject: "shop"},
			{Name: "api", Status: "running", CPUPerc: 20, ComposeProject: "shop"},
		},
	}

	problems := m.problems()
	for _, kind := range []string{
		"Noisy neighbor",
		"Log offender",
		"Build cache offender",
		"Unused tagged images",
		"Dangling images",
		"Stopped container storage",
		"Unused volume storage",
		"Host storage gap",
	} {
		if !hasProblemKind(problems, kind) {
			t.Fatalf("missing %q in %+v", kind, problems)
		}
	}
}

func hasProblemKind(problems []Problem, kind string) bool {
	for _, p := range problems {
		if p.Kind == kind {
			return true
		}
	}
	return false
}
