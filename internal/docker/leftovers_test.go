package docker

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/volume"
)

// leftoverFixture is testdata/leftovers-desktop-4.100.1.json: the Phase 1
// scenario snapshot from a real macOS Docker Desktop daemon.
type leftoverFixture struct {
	GeneratedAt time.Time            `json:"generatedAt"`
	Containers  []*container.Summary `json:"containers"`
	Volumes     []*volume.Volume     `json:"volumes"`
	Inspects    []struct {
		ID    string `json:"Id"`
		State struct {
			Status     string `json:"Status"`
			FinishedAt string `json:"FinishedAt"`
		} `json:"State"`
	} `json:"inspects"`
}

func loadLeftoverFixture(t *testing.T) (leftoverFixture, []LeftoverContainer, []LeftoverVolume) {
	t.Helper()
	data, err := os.ReadFile("testdata/leftovers-desktop-4.100.1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f leftoverFixture
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	// Mirror ListLeftovers: only stopped containers get a stop time.
	finished := map[string]time.Time{}
	for _, in := range f.Inspects {
		if in.State.Status == "exited" || in.State.Status == "dead" {
			if ts := parseFinishedAt(in.State.FinishedAt); !ts.IsZero() {
				finished[in.ID] = ts
			}
		}
	}
	return f, leftoverContainersFromSDK(f.Containers, finished), leftoverVolumesFromSDK(f.Volumes)
}

// fixtureScope is the compose file the scenario ended with (alive-v2.yaml):
// service "worker" and volume "cache" were removed from it.
func fixtureScope() *ComposeScope {
	return &ComposeScope{
		Project:  "dvzleft",
		Services: map[string]bool{"web": true},
		Volumes:  map[string]bool{"data": true},
	}
}

type leftoverSummary struct {
	Kind       LeftoverKind
	Name       string
	Confidence LeftoverConfidence
	Removable  bool
	HeldBy     string
}

// summarize sorts by Kind string, then Name, so expectations are stable.
func summarize(ls []Leftover) []leftoverSummary {
	out := make([]leftoverSummary, 0, len(ls))
	for _, l := range ls {
		name := l.Name
		if l.Kind == LeftoverAnonymousVolume {
			name = "<anonymous>"
		}
		if strings.HasPrefix(name, "dvzleft-web-run-") {
			name = "dvzleft-web-run-*"
		}
		out = append(out, leftoverSummary{l.Kind, name, l.Confidence, l.Removable, strings.Join(l.HeldBy, ",")})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func TestDetectLeftoversDockerDesktopFixture(t *testing.T) {
	f, cs, vs := loadLeftoverFixture(t)

	got := summarize(DetectLeftovers(LeftoverInput{
		Containers: cs,
		Volumes:    vs,
		Compose:    fixtureScope(),
		Now:        f.GeneratedAt.Add(30 * 24 * time.Hour),
	}))
	want := []leftoverSummary{
		{LeftoverAnonymousVolume, "<anonymous>", ConfidenceCertain, true, ""},
		{LeftoverBuilderCache, "dvz-leftovers", ConfidenceReport, false, ""},
		// Held by the running orphan: not removable until it goes.
		{LeftoverDroppedVolume, "dvzleft_cache", ConfidenceCertain, false, "dvzleft-worker-1"},
		{LeftoverComposeOneOff, "dvzleft-web-run-*", ConfidenceCertain, true, ""},
		// Still running in the real run, so guidance only.
		{LeftoverComposeOrphan, "dvzleft-worker-1", ConfidenceCertain, false, ""},
		{LeftoverOwnerlessVolume, "dvzgone_dbdata", ConfidenceLikely, true, ""},
		{LeftoverStoppedContainer, "dvz-leftovers-stopped", ConfidenceLikely, true, ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("leftovers mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestDetectLeftoversFixtureWithoutComposeFile(t *testing.T) {
	f, cs, vs := loadLeftoverFixture(t)

	got := summarize(DetectLeftovers(LeftoverInput{
		Containers: cs,
		Volumes:    vs,
		Now:        f.GeneratedAt, // just after the run: nothing is "long stopped"
	}))
	// Orphan and dropped-volume rules need the compose file. dvzleft_* volumes
	// belong to a project that still has containers, so they are not ownerless.
	want := []leftoverSummary{
		{LeftoverAnonymousVolume, "<anonymous>", ConfidenceCertain, true, ""},
		{LeftoverBuilderCache, "dvz-leftovers", ConfidenceReport, false, ""},
		{LeftoverComposeOneOff, "dvzleft-web-run-*", ConfidenceCertain, true, ""},
		{LeftoverOwnerlessVolume, "dvzgone_dbdata", ConfidenceLikely, true, ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("leftovers mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestLeftoverSDKConversionFromFixture(t *testing.T) {
	_, cs, vs := loadLeftoverFixture(t)

	byName := map[string]LeftoverContainer{}
	for _, c := range cs {
		byName[c.Name] = c
	}
	worker, ok := byName["dvzleft-worker-1"]
	if !ok || worker.State != "running" || !reflect.DeepEqual(worker.Volumes, []string{"dvzleft_cache"}) || !worker.FinishedAt.IsZero() {
		t.Fatalf("worker = %+v", worker)
	}
	stopped := byName["dvz-leftovers-stopped"]
	if stopped.FinishedAt.IsZero() || !stopped.SizeKnown {
		t.Fatalf("stopped container = %+v, want FinishedAt and size", stopped)
	}

	for _, v := range vs {
		if !v.SizeKnown || v.RefCount < 0 {
			t.Fatalf("volume %s: usage data lost in conversion: %+v", v.Name, v)
		}
	}
}

func TestDetectLeftoversRules(t *testing.T) {
	now := time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)
	scope := &ComposeScope{Project: "shop", Services: map[string]bool{"api": true}, Volumes: map[string]bool{"data": true}}
	compose := func(project, service, oneoff string) map[string]string {
		return map[string]string{labelComposeProject: project, labelComposeService: service, labelComposeOneOff: oneoff}
	}
	vol := func(project, key string) map[string]string {
		return map[string]string{labelComposeProject: project, labelComposeVolume: key}
	}

	tests := []struct {
		name  string
		in    LeftoverInput
		scope *ComposeScope
		want  []leftoverSummary
	}{
		{
			name: "running one-off is an active session, not a leftover",
			in:   LeftoverInput{Containers: []LeftoverContainer{{Name: "shop-api-run-1", State: "running", Labels: compose("shop", "api", "True")}}},
		},
		{
			name:  "stopped orphan is removable",
			scope: scope,
			in:    LeftoverInput{Containers: []LeftoverContainer{{Name: "shop-old-1", State: "exited", Labels: compose("shop", "old", "False")}}},
			want:  []leftoverSummary{{LeftoverComposeOrphan, "shop-old-1", ConfidenceCertain, true, ""}},
		},
		{
			name:  "container of another project is never an orphan of this one",
			scope: scope,
			in:    LeftoverInput{Containers: []LeftoverContainer{{Name: "blog-web-1", State: "running", Labels: compose("blog", "web", "False")}}},
		},
		{
			name: "attached anonymous volume is in use",
			in: LeftoverInput{
				Containers: []LeftoverContainer{{Name: "app", State: "running", Volumes: []string{"abc"}}},
				Volumes:    []LeftoverVolume{{Name: "abc", Labels: map[string]string{labelAnonymousVol: ""}, RefCount: -1}},
			},
		},
		{
			name: "plain named volume without compose labels is left to Local Volumes",
			in:   LeftoverInput{Volumes: []LeftoverVolume{{Name: "mydata", RefCount: 0}}},
		},
		{
			name:  "volume still defined by the current project is kept data",
			scope: scope,
			in:    LeftoverInput{Volumes: []LeftoverVolume{{Name: "shop_data", Labels: vol("shop", "data"), RefCount: 0}}},
		},
		{
			name:  "dropped volume with nothing attached is removable",
			scope: scope,
			in:    LeftoverInput{Volumes: []LeftoverVolume{{Name: "shop_cache", Labels: vol("shop", "cache"), RefCount: 0}}},
			want:  []leftoverSummary{{LeftoverDroppedVolume, "shop_cache", ConfidenceCertain, true, ""}},
		},
		{
			name: "ownerless volume still attached (refCount) is not reported",
			in:   LeftoverInput{Volumes: []LeftoverVolume{{Name: "gone_db", Labels: vol("gone", "db"), RefCount: 1}}},
		},
		{
			name: "volume of a project with stopped containers is not ownerless",
			in: LeftoverInput{
				Containers: []LeftoverContainer{{Name: "gone-db-1", State: "exited", Labels: compose("gone", "db", "False")}},
				Volumes:    []LeftoverVolume{{Name: "gone_db", Labels: vol("gone", "db"), RefCount: 0}},
			},
		},
		{
			name: "created-but-never-started container has no stop time",
			in:   LeftoverInput{Containers: []LeftoverContainer{{Name: "never", State: "created"}}},
		},
		{
			name: "stop threshold is inclusive and configurable",
			in: LeftoverInput{
				StoppedFor: 7 * 24 * time.Hour,
				Containers: []LeftoverContainer{
					{Name: "seven", State: "exited", FinishedAt: now.Add(-7 * 24 * time.Hour)},
					{Name: "six", State: "exited", FinishedAt: now.Add(-6 * 24 * time.Hour)},
				},
			},
			want: []leftoverSummary{{LeftoverStoppedContainer, "seven", ConfidenceLikely, true, ""}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Now = now
			tt.in.Compose = tt.scope
			got := summarize(DetectLeftovers(tt.in))
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("\n got: %+v\nwant: %+v", got, tt.want)
			}
		})
	}
}

func TestDetectLeftoversOrdering(t *testing.T) {
	now := time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)
	got := DetectLeftovers(LeftoverInput{
		Now: now,
		Volumes: []LeftoverVolume{
			{Name: "buildx_buildkit_b0_state", SizeMB: 9000, RefCount: 1},
			{Name: "gone_big", Labels: map[string]string{labelComposeProject: "gone", labelComposeVolume: "big"}, SizeMB: 800},
			{Name: "anon-small", Labels: map[string]string{labelAnonymousVol: ""}, SizeMB: 10},
			{Name: "anon-big", Labels: map[string]string{labelAnonymousVol: ""}, SizeMB: 500},
		},
	})
	var names []string
	for _, l := range got {
		names = append(names, l.ID)
	}
	want := []string{"anon-big", "anon-small", "gone_big", "buildx_buildkit_b0_state"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("order = %v, want %v (certain, likely, report; then size)", names, want)
	}
}

func TestBuildxNames(t *testing.T) {
	tests := []struct {
		volume, builder string
		ok              bool
	}{
		{"buildx_buildkit_dvz-leftovers0_state", "dvz-leftovers", true},
		{"buildx_buildkit_b20_state", "b2", true},
		{"buildx_buildkit_multi1_state", "multi", true},
		{"buildx_buildkit__state", "", false},
		{"myapp_state", "", false},
	}
	for _, tt := range tests {
		node, ok := buildxStateNode(tt.volume)
		if ok != tt.ok {
			t.Fatalf("%s: ok = %v", tt.volume, ok)
		}
		if ok && buildxBuilderName(node) != tt.builder {
			t.Fatalf("%s: builder = %q, want %q", tt.volume, buildxBuilderName(node), tt.builder)
		}
	}
}

func TestParseFinishedAt(t *testing.T) {
	if !parseFinishedAt("0001-01-01T00:00:00Z").IsZero() {
		t.Fatal("Docker's never-stopped value must stay zero")
	}
	if !parseFinishedAt("").IsZero() || !parseFinishedAt("garbage").IsZero() {
		t.Fatal("unparseable values must be zero")
	}
	if parseFinishedAt("2026-11-27T05:11:48.187226212Z").IsZero() {
		t.Fatal("real value parsed as zero")
	}
}

func TestDemoClientListLeftoversCoversEveryKind(t *testing.T) {
	ls, err := NewDemoClient().ListLeftovers(&ComposeScope{
		Project:  "demo-stack",
		Services: map[string]bool{"nginx": true, "api": true, "postgres": true, "redis": true, "worker": true, "migration": true},
		Volumes:  map[string]bool{"pgdata": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[LeftoverKind]bool{}
	for _, l := range ls {
		seen[l.Kind] = true
		if l.Name == "postgres-db" || l.Name == "demo-stack_pgdata" {
			t.Fatalf("in-use demo resource reported: %+v", l)
		}
	}
	for _, k := range []LeftoverKind{
		LeftoverComposeOrphan, LeftoverComposeOneOff, LeftoverDroppedVolume, LeftoverOwnerlessVolume,
		LeftoverAnonymousVolume, LeftoverStoppedContainer, LeftoverBuilderCache,
	} {
		if !seen[k] {
			t.Errorf("demo leftovers missing kind %s", k)
		}
	}
}

func TestCheckRemovable(t *testing.T) {
	listed := Leftover{Kind: LeftoverOwnerlessVolume, ID: "gone_db", Name: "gone_db", Removable: true, SizeMB: 10}

	if got, err := CheckRemovable(listed, []Leftover{listed}); err != nil || got.ID != "gone_db" {
		t.Fatalf("unchanged removable item: got %+v err %v", got, err)
	}

	// The same volume is now attached (e.g. the project came back up).
	held := listed
	held.Removable = false
	held.Action = "still used"
	if _, err := CheckRemovable(listed, []Leftover{held}); !errors.Is(err, ErrLeftoverChanged) {
		t.Fatalf("attached since listing: err = %v, want ErrLeftoverChanged", err)
	}

	// Gone, or re-classified under another kind with the same ID.
	if _, err := CheckRemovable(listed, nil); !errors.Is(err, ErrLeftoverChanged) {
		t.Fatalf("no longer detected: err = %v", err)
	}
	other := listed
	other.Kind = LeftoverDroppedVolume
	if _, err := CheckRemovable(listed, []Leftover{other}); !errors.Is(err, ErrLeftoverChanged) {
		t.Fatalf("kind changed: err = %v", err)
	}

	cache := Leftover{Kind: LeftoverBuilderCache, ID: "buildx_buildkit_b0_state", Name: "b"}
	if _, err := CheckRemovable(cache, []Leftover{cache}); err == nil || errors.Is(err, ErrLeftoverChanged) {
		t.Fatalf("buildx cache must be refused as info-only, err = %v", err)
	}
}

func TestDemoClientRemoveLeftover(t *testing.T) {
	d := NewDemoClient()
	before, _ := d.ListLeftovers(nil)
	var target, held Leftover
	for _, l := range before {
		if l.Kind == LeftoverAnonymousVolume {
			target = l
		}
	}
	if _, err := d.RemoveLeftover(target, nil); err != nil {
		t.Fatal(err)
	}
	after, _ := d.ListLeftovers(nil)
	for _, l := range after {
		if l.ID == target.ID {
			t.Fatal("removed demo leftover still listed")
		}
	}
	if _, err := d.RemoveLeftover(target, nil); !errors.Is(err, ErrLeftoverChanged) {
		t.Fatalf("second removal: err = %v, want ErrLeftoverChanged", err)
	}

	scope := &ComposeScope{Project: "demo-stack", Services: map[string]bool{"postgres": true}, Volumes: map[string]bool{"pgdata": true}}
	ls, _ := d.ListLeftovers(scope)
	for _, l := range ls {
		if l.Kind == LeftoverDroppedVolume {
			held = l
		}
	}
	if _, err := d.RemoveLeftover(held, scope); err == nil {
		t.Fatal("volume held by a running orphan must not be removable")
	}
}
