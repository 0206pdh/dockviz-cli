package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	composectx "github.com/0206pdh/dockviz-cli/internal/compose"
	"github.com/0206pdh/dockviz-cli/internal/docker"
	tea "github.com/charmbracelet/bubbletea"
)

// sampleLeftovers mirrors what the real Docker Desktop run detected
// (docs/validation.ko.md §10), in DetectLeftovers order.
func sampleLeftovers() []docker.Leftover {
	return []docker.Leftover{
		{Kind: docker.LeftoverDroppedVolume, Confidence: docker.ConfidenceCertain, ID: "dvzleft_cache", Name: "dvzleft_cache",
			Project: "dvzleft", SizeMB: 4, SizeKnown: true, HeldBy: []string{"dvzleft-worker-1"},
			Reason: `volume "cache" is no longer in the compose file`, Action: "still used; remove the container(s) holding it first"},
		{Kind: docker.LeftoverAnonymousVolume, Confidence: docker.ConfidenceCertain, ID: "85de3eb1585d", Name: "85de3eb1585d",
			SizeMB: 3, SizeKnown: true, Removable: true, Reason: "anonymous volume no container uses", Action: "remove it"},
		{Kind: docker.LeftoverComposeOrphan, Confidence: docker.ConfidenceCertain, ID: "c0ffee", Name: "dvzleft-worker-1",
			Project: "dvzleft", Service: "worker", SizeKnown: true, Reason: `service "worker" is no longer in the compose file`,
			Action: "still running; stop it or run `docker compose up --remove-orphans`"},
		{Kind: docker.LeftoverOwnerlessVolume, Confidence: docker.ConfidenceLikely, ID: "dvzgone_dbdata", Name: "dvzgone_dbdata",
			Project: "dvzgone", SizeMB: 4, SizeKnown: true, Removable: true, Reason: "no container of compose project", Action: "remove it if the project is gone"},
		{Kind: docker.LeftoverBuilderCache, Confidence: docker.ConfidenceReport, ID: "buildx_buildkit_dvz-leftovers0_state", Name: "dvz-leftovers",
			SizeMB: 8.4, SizeKnown: true, Reason: "build cache of buildx builder", Action: "`docker buildx prune --builder dvz-leftovers`"},
	}
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		updated, _ := m.Update(keyMsg(k))
		m = updated.(Model)
	}
	return m
}

func leftoverModel() Model {
	return Model{
		keys:            DefaultKeyMap(),
		activePanel:     PanelDiskUsage,
		diskUsageLoaded: true,
		leftoversLoaded: true,
		leftovers:       sampleLeftovers(),
	}
}

func TestRenderDiskUsageLeftoversRow(t *testing.T) {
	view := leftoverModel().renderDiskUsage()
	// 5 findings, 2 not removable (held volume, running orphan) + 1 info.
	for _, want := range []string{"Leftovers", "certain 3 · likely 1 · info 1", "[Enter] review"} {
		if !strings.Contains(view, want) {
			t.Errorf("renderDiskUsage() missing %q in:\n%s", want, view)
		}
	}
	// TOTAL 5, ACTIVE 3 (held volume, running orphan, info-only cache),
	// SIZE 19 MB, RECLAIMABLE 7 MB (anonymous 3 MB + ownerless 4 MB).
	var row string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Leftovers") && !strings.Contains(line, "certain") {
			row = line
		}
	}
	if f := strings.Fields(row); len(f) < 7 || f[1] != "5" || f[2] != "3" || f[3]+f[4] != "19MB" || f[5]+f[6] != "7MB" {
		t.Errorf("Leftovers row = %q, want TOTAL 5 ACTIVE 3 SIZE 19 MB RECLAIMABLE 7 MB", row)
	}
}

func TestRenderDiskUsageLeftoversStates(t *testing.T) {
	m := Model{activePanel: PanelDiskUsage, diskUsageLoaded: true}
	if v := m.renderDiskUsage(); !strings.Contains(v, "Leftovers") || !strings.Contains(v, "…") {
		t.Errorf("not-yet-loaded leftovers should show a placeholder:\n%s", v)
	}
	m.leftoversLoaded = true
	if v := m.renderDiskUsage(); !strings.Contains(v, "no leftover resources found") {
		t.Errorf("empty leftovers message missing:\n%s", v)
	}
	m.leftoversErr = errors.New("system/df failed")
	if v := m.renderDiskUsage(); !strings.Contains(v, "leftover detection failed: system/df failed") {
		t.Errorf("leftover error missing:\n%s", v)
	}
}

func TestLeftoversViewNavigation(t *testing.T) {
	m := leftoverModel()
	m.cursor = leftoversRowIndex()

	m = press(t, m, "enter")
	if m.activeView != ViewLeftovers {
		t.Fatalf("Enter on Leftovers row: activeView = %v, want ViewLeftovers", m.activeView)
	}

	m = press(t, m, "up", "down", "down", "down", "down", "down", "down")
	if m.leftoverCursor != len(sampleLeftovers())-1 {
		t.Fatalf("leftoverCursor = %d, want clamped to last item", m.leftoverCursor)
	}
	m = press(t, m, "esc")
	if m.activeView != ViewDashboard || m.cursor != leftoversRowIndex() {
		t.Fatalf("Esc should return to Disk Usage with the Leftovers row selected (view=%v cursor=%d)", m.activeView, m.cursor)
	}
}

func TestLeftoversRowIsNotAPruneCategory(t *testing.T) {
	m := leftoverModel()
	m.cursor = leftoversRowIndex()
	m = press(t, m, "d")
	if m.confirmDelete {
		t.Fatal("[d] on Leftovers opened a prune confirmation")
	}
	if m.diskUsageErr == nil || !strings.Contains(m.diskUsageErr.Error(), "press Enter") {
		t.Fatalf("diskUsageErr = %v, want guidance to press Enter", m.diskUsageErr)
	}
}

func TestDownReachesLeftoversRow(t *testing.T) {
	m := leftoverModel()
	for i := 0; i < 10; i++ {
		m = press(t, m, "down")
	}
	if m.cursor != leftoversRowIndex() {
		t.Fatalf("cursor = %d, want Leftovers row %d", m.cursor, leftoversRowIndex())
	}
}

func TestRenderLeftoversView(t *testing.T) {
	m := leftoverModel()
	m.activeView = ViewLeftovers
	m.leftoverCursor = 0

	view := m.View()
	for _, want := range []string{
		"read-only", "no compose file loaded",
		"dropped volume", "held by dvzleft-worker-1",
		"anonymous volume", "removable",
		"compose orphan", "in use",
		"ownerless volume", "buildx cache", "info only",
		// detail of the selected (first) item
		"Reason", `volume "cache" is no longer in the compose file`, "Action", "Held by",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("renderLeftovers() missing %q in:\n%s", want, view)
		}
	}

	m.composeContext = &composectx.Context{ProjectName: "dvzleft", Files: []string{"/tmp/proj/compose.yaml"}}
	if v := m.renderLeftovers(); !strings.Contains(v, `compose project "dvzleft"`) {
		t.Errorf("compose scope line missing:\n%s", v)
	}

	m.leftovers = nil
	if v := m.renderLeftovers(); !strings.Contains(v, "No leftover resources found") {
		t.Errorf("empty state missing:\n%s", v)
	}
}

func TestLeftoversViewScrollsLongLists(t *testing.T) {
	m := leftoverModel()
	m.activeView = ViewLeftovers
	m.height = 20 // 4 list rows
	for i := 0; i < 20; i++ {
		m.leftovers = append(m.leftovers, docker.Leftover{Kind: docker.LeftoverAnonymousVolume, Name: "vol" + string(rune('a'+i)), SizeKnown: true, Removable: true})
	}
	m.leftoverCursor = 12
	view := m.renderLeftovers()
	if !strings.Contains(view, "of 25") {
		t.Errorf("scroll indicator missing:\n%s", view)
	}
}

func TestDiskUsageMsgAppliesLeftovers(t *testing.T) {
	m := Model{leftoverCursor: 9}
	updated, _ := m.Update(diskUsageMsg{leftovers: sampleLeftovers()})
	got := updated.(Model)
	if !got.leftoversLoaded || len(got.leftovers) != 5 || got.leftoverCursor != 4 {
		t.Fatalf("leftovers not applied/clamped: loaded=%v n=%d cursor=%d", got.leftoversLoaded, len(got.leftovers), got.leftoverCursor)
	}

	// A later detection failure keeps the last good list and shows the error.
	updated, _ = got.Update(diskUsageMsg{leftoversErr: errors.New("boom")})
	got = updated.(Model)
	if len(got.leftovers) != 5 || got.leftoversErr == nil {
		t.Fatalf("failed detection should keep the previous list: n=%d err=%v", len(got.leftovers), got.leftoversErr)
	}
}

func TestLeftoverProblemSummary(t *testing.T) {
	m := Model{leftoversLoaded: true, leftovers: sampleLeftovers()}
	var found *Problem
	for _, p := range m.problems() {
		if p.Kind == "Leftover resources" {
			p := p
			found = &p
		}
	}
	if found == nil {
		t.Fatal("Problems has no leftover summary")
	}
	if found.Severity != severityInfo || !strings.Contains(found.Detail, "certain 3, likely 1") {
		t.Fatalf("summary = %+v", *found)
	}

	big := Model{leftoversLoaded: true, leftovers: []docker.Leftover{
		{Kind: docker.LeftoverOwnerlessVolume, Confidence: docker.ConfidenceLikely, SizeMB: 2048, SizeKnown: true, Removable: true},
	}}
	if ps := big.leftoverProblems(timeNowForTest()); len(ps) != 1 || ps[0].Severity != severityWarning {
		t.Fatalf("1GB+ removable should warn: %+v", ps)
	}

	infoOnly := Model{leftoversLoaded: true, leftovers: []docker.Leftover{
		{Kind: docker.LeftoverBuilderCache, Confidence: docker.ConfidenceReport, SizeMB: 9000, SizeKnown: true},
	}}
	if ps := infoOnly.leftoverProblems(timeNowForTest()); len(ps) != 0 {
		t.Fatalf("info-only leftovers should not raise a problem: %+v", ps)
	}
}

func TestVolumePruneWarningNamesComposeVolumes(t *testing.T) {
	m := Model{
		keys:            DefaultKeyMap(),
		activePanel:     PanelDiskUsage,
		diskUsageLoaded: true,
		diskUsage: docker.DiskUsageInfo{
			Volumes: docker.DiskUsageCategory{Total: 3, SizeMB: 300, ReclaimMB: 200, ComposeUnattached: 2, ComposeUnattachedMB: 150},
		},
	}
	m.cursor = 2 // Local Volumes
	m = press(t, m, "d")
	if !m.confirmDelete {
		t.Fatal("volume prune confirmation did not open")
	}
	view := m.View()
	for _, want := range []string{"2 of these (150 MB)", "belong to Compose", "Check Leftovers first"} {
		if !strings.Contains(view, want) {
			t.Errorf("volume prune dialog missing %q:\n%s", want, view)
		}
	}

	if w := pruneWarning(diskUsageRow{key: "volumes", warning: "base"}); w != "base" {
		t.Fatalf("no compose volumes should keep the base warning, got %q", w)
	}
}

func TestLeftoverScopeFromComposeContext(t *testing.T) {
	if (Model{}).leftoverScope() != nil {
		t.Fatal("no compose file should give a nil scope")
	}
	m := Model{composeContext: &composectx.Context{
		ProjectName: "shop",
		Services:    map[string]composectx.ServiceContext{"api": {}, "db": {}},
		Volumes:     []string{"data"},
	}}
	s := m.leftoverScope()
	if s.Project != "shop" || !s.Services["api"] || !s.Services["db"] || !s.Volumes["data"] || s.Services["worker"] {
		t.Fatalf("scope = %+v", s)
	}
}

// fakeDiskClient implements only what fetchDiskUsageCmd calls; the embedded
// interface panics if anything else is used.
type fakeDiskClient struct {
	docker.DockerClient
	gotScope *docker.ComposeScope
	duErr    error
	leftErr  error
}

func (f *fakeDiskClient) DiskUsage() (docker.DiskUsageInfo, error) {
	return docker.DiskUsageInfo{Volumes: docker.DiskUsageCategory{ComposeUnattached: 1}}, f.duErr
}

func (f *fakeDiskClient) ListLeftovers(scope *docker.ComposeScope) ([]docker.Leftover, error) {
	f.gotScope = scope
	if f.leftErr != nil {
		return nil, f.leftErr
	}
	return sampleLeftovers(), nil
}

func TestFetchDiskUsageCmdIncludesLeftovers(t *testing.T) {
	fc := &fakeDiskClient{}
	scope := &docker.ComposeScope{Project: "shop"}
	msg := fetchDiskUsageCmd(fc, scope)().(diskUsageMsg)
	if msg.err != nil || msg.leftoversErr != nil || len(msg.leftovers) != 5 || msg.info.Volumes.ComposeUnattached != 1 {
		t.Fatalf("msg = %+v", msg)
	}
	if fc.gotScope != scope {
		t.Fatal("compose scope was not passed to ListLeftovers")
	}

	// Leftover detection failing must not hide the disk usage breakdown.
	fc = &fakeDiskClient{leftErr: errors.New("boom")}
	msg = fetchDiskUsageCmd(fc, nil)().(diskUsageMsg)
	if msg.err != nil || msg.leftoversErr == nil || msg.info.Volumes.ComposeUnattached != 1 {
		t.Fatalf("msg = %+v", msg)
	}
}

func TestListWindow(t *testing.T) {
	tests := []struct{ total, cursor, rows, start, end int }{
		{5, 0, 10, 0, 5},
		{20, 0, 4, 0, 4},
		{20, 10, 4, 8, 12},
		{20, 19, 4, 16, 20},
	}
	for _, tt := range tests {
		s, e := listWindow(tt.total, tt.cursor, tt.rows)
		if s != tt.start || e != tt.end {
			t.Errorf("listWindow(%d,%d,%d) = %d,%d want %d,%d", tt.total, tt.cursor, tt.rows, s, e, tt.start, tt.end)
		}
		if tt.cursor < s || tt.cursor >= e {
			t.Errorf("cursor %d outside window [%d,%d)", tt.cursor, s, e)
		}
	}
}

func timeNowForTest() time.Time { return time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC) }
