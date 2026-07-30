package tui

import (
	"testing"

	"github.com/0206pdh/dockviz-cli/internal/docker"
)

func TestSummarizeProjects(t *testing.T) {
	containers := []docker.ContainerInfo{
		{
			Name:           "api-1",
			Status:         "running",
			CPUPerc:        20,
			MemMB:          256,
			CPULimit:       1,
			MemoryLimitMB:  512,
			LimitsKnown:    true,
			ComposeProject: "shop",
			ComposeService: "api",
		},
		{
			Name:           "worker-1",
			Status:         "running",
			CPUPerc:        90,
			MemMB:          1024,
			ComposeProject: "shop",
			ComposeService: "worker",
			LimitsKnown:    true,
		},
		{
			Name:    "standalone",
			Status:  "running",
			CPUPerc: 5,
			MemMB:   128,
		},
	}

	projects := summarizeProjects(containers)
	if len(projects) != 2 {
		t.Fatalf("summarizeProjects() returned %d projects, want 2: %+v", len(projects), projects)
	}
	if projects[0].Name != "shop" {
		t.Fatalf("first project = %q, want shop: %+v", projects[0].Name, projects)
	}
	if projects[0].CPU != 110 || projects[0].MemoryMB != 1280 {
		t.Fatalf("shop resources = CPU %v MEM %v, want 110 and 1280", projects[0].CPU, projects[0].MemoryMB)
	}
	if projects[0].TopContainer != "worker-1" {
		t.Fatalf("top container = %q, want worker-1", projects[0].TopContainer)
	}
	if projects[0].Unbounded != 1 {
		t.Fatalf("unbounded = %d, want 1", projects[0].Unbounded)
	}
}

func TestHasProjectContext(t *testing.T) {
	if hasProjectContext(nil) {
		t.Fatal("hasProjectContext(nil) = true, want false")
	}
	if hasProjectContext([]docker.ContainerInfo{{Name: "api"}}) {
		t.Fatal("hasProjectContext(standalone) = true, want false")
	}
	if !hasProjectContext([]docker.ContainerInfo{{Name: "api", ComposeProject: "shop"}}) {
		t.Fatal("hasProjectContext(compose) = false, want true")
	}
}
