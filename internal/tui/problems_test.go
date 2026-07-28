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
