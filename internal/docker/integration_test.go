package docker

// Real-daemon integration tests (T-007). They run only with
// DOCKVIZ_INTEGRATION=1, create their own uniquely named resources through
// the docker CLI (as a user would), and remove everything in t.Cleanup.
//
//	DOCKVIZ_INTEGRATION=1 go test -run Integration -count=1 -v ./internal/docker/
//
// CI runs them on ubuntu-latest (.github/workflows/ci.yml, job "integration").

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
)

const integrationImage = "busybox:1.37"

type integrationEnv struct {
	t   *testing.T
	dc  *Client
	dir string
}

func newIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()
	if os.Getenv("DOCKVIZ_INTEGRATION") != "1" {
		t.Skip("set DOCKVIZ_INTEGRATION=1 to run real-daemon integration tests")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker CLI not found: %v", err)
	}
	dc, err := NewClient("", "")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(dc.Close)

	// Pull through the SDK so the CLI never needs registry credentials.
	rc, err := dc.cli.ImagePull(dc.ctx, integrationImage, image.PullOptions{})
	if err != nil {
		t.Fatalf("pull %s: %v", integrationImage, err)
	}
	_, _ = io.Copy(io.Discard, rc)
	rc.Close()

	return &integrationEnv{t: t, dc: dc, dir: t.TempDir()}
}

// docker runs the CLI against the same daemon as the SDK client.
func (e *integrationEnv) docker(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	cmd.Dir = e.dir
	cmd.Env = append(os.Environ(), "DOCKER_HOST="+e.dc.daemonHost)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	out := strings.TrimSpace(buf.String())
	if err != nil {
		return out, fmt.Errorf("docker %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

func (e *integrationEnv) mustDocker(args ...string) string {
	e.t.Helper()
	out, err := e.docker(args...)
	if err != nil {
		e.t.Fatal(err)
	}
	return out
}

func (e *integrationEnv) writeCompose(name string, services, volumes []string) {
	e.t.Helper()
	var b strings.Builder
	b.WriteString("services:\n")
	for _, s := range services {
		vol := volumes[0]
		if s == "worker" && len(volumes) > 1 {
			vol = volumes[1]
		}
		fmt.Fprintf(&b, "  %s:\n    image: %s\n    labels:\n      dockviz.test: integration\n    command: [\"sleep\", \"600\"]\n    volumes:\n      - %s:/data\n",
			s, integrationImage, vol)
	}
	b.WriteString("volumes:\n")
	for _, v := range volumes {
		fmt.Fprintf(&b, "  %s:\n    labels:\n      dockviz.test: integration\n", v)
	}
	if err := os.WriteFile(filepath.Join(e.dir, name), []byte(b.String()), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// TestIntegrationLeftoversDetectAndRemove builds every removable leftover
// kind on a real daemon, checks detection, then removes them one by one
// through RemoveLeftover and verifies nothing of this run is left.
func TestIntegrationLeftoversDetectAndRemove(t *testing.T) {
	e := newIntegrationEnv(t)

	id := fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
	alive, gone := "dvzit"+id+"a", "dvzit"+id+"g"
	e.writeCompose("v1.yaml", []string{"web", "worker"}, []string{"data", "cache"})
	e.writeCompose("v2.yaml", []string{"web"}, []string{"data"})
	e.writeCompose("gone.yaml", []string{"db"}, []string{"dbdata"})

	var anonVolume string
	t.Cleanup(func() {
		_, _ = e.docker("compose", "-p", alive, "-f", "v1.yaml", "down", "-v", "--remove-orphans")
		_, _ = e.docker("compose", "-p", gone, "-f", "gone.yaml", "down", "-v", "--remove-orphans")
		if anonVolume != "" {
			_, _ = e.docker("volume", "rm", "-f", anonVolume)
		}
	})

	// Compose: one-off, orphan (stopped so it is removable), dropped volume.
	e.mustDocker("compose", "-p", alive, "-f", "v1.yaml", "up", "-d", "--wait")
	e.mustDocker("compose", "-p", alive, "-f", "v1.yaml", "run", "web", "true")
	e.mustDocker("compose", "-p", alive, "-f", "v2.yaml", "up", "-d", "--wait")
	e.mustDocker("stop", "-t", "1", alive+"-worker-1")
	// A project brought down without -v.
	e.mustDocker("compose", "-p", gone, "-f", "gone.yaml", "up", "-d", "--wait")
	e.mustDocker("compose", "-p", gone, "-f", "gone.yaml", "down")
	// An anonymous volume left by `docker rm` without -v.
	ctr := e.mustDocker("create", "--label", "dockviz.test=integration", "-v", "/data", integrationImage, "true")
	insp, err := e.dc.cli.ContainerInspect(e.dc.ctx, ctr)
	if err != nil || len(insp.Mounts) != 1 {
		t.Fatalf("inspect anonymous-volume container: %v mounts=%v", err, insp.Mounts)
	}
	anonVolume = insp.Mounts[0].Name
	e.mustDocker("rm", ctr)

	scope := &ComposeScope{Project: alive, Services: map[string]bool{"web": true}, Volumes: map[string]bool{"data": true}}
	ours := func(l Leftover) bool {
		return l.Project == alive || l.Project == gone || l.ID == anonVolume
	}
	list := func() map[LeftoverKind]Leftover {
		t.Helper()
		ls, err := e.dc.ListLeftovers(scope)
		if err != nil {
			t.Fatalf("ListLeftovers: %v", err)
		}
		out := map[LeftoverKind]Leftover{}
		for _, l := range ls {
			if ours(l) {
				if _, dup := out[l.Kind]; dup {
					t.Fatalf("two %s leftovers from this run: %+v", l.Kind, ls)
				}
				out[l.Kind] = l
			}
		}
		return out
	}
	expect := func(got map[LeftoverKind]Leftover, kind LeftoverKind, name string, removable bool) Leftover {
		t.Helper()
		l, ok := got[kind]
		if !ok {
			t.Fatalf("%s not detected; got %+v", kind, got)
		}
		if name != "" && l.Name != name {
			t.Fatalf("%s name = %q, want %q", kind, l.Name, name)
		}
		if l.Removable != removable {
			t.Fatalf("%s %s removable = %v, want %v (%s)", kind, l.Name, l.Removable, removable, l.Action)
		}
		return l
	}

	got := list()
	orphan := expect(got, LeftoverComposeOrphan, alive+"-worker-1", true)
	oneoff := expect(got, LeftoverComposeOneOff, "", true)
	dropped := expect(got, LeftoverDroppedVolume, alive+"_cache", false)
	ownerless := expect(got, LeftoverOwnerlessVolume, gone+"_dbdata", true)
	anon := expect(got, LeftoverAnonymousVolume, anonVolume, true)
	if len(dropped.HeldBy) != 1 || dropped.HeldBy[0] != alive+"-worker-1" {
		t.Fatalf("dropped volume HeldBy = %v, want the stopped orphan", dropped.HeldBy)
	}
	if len(got) != 5 {
		t.Fatalf("unexpected leftovers from this run: %+v", got)
	}

	// Refused: the dropped volume is still held by the orphan.
	if _, err := e.dc.RemoveLeftover(dropped, scope); !errors.Is(err, ErrLeftoverChanged) {
		t.Fatalf("removing a held volume: err = %v, want ErrLeftoverChanged", err)
	}
	if _, err := e.docker("volume", "inspect", alive+"_cache"); err != nil {
		t.Fatalf("held volume was removed: %v", err)
	}

	for _, l := range []Leftover{oneoff, orphan} {
		if _, err := e.dc.RemoveLeftover(l, scope); err != nil {
			t.Fatalf("remove %s %s: %v", l.Kind, l.Name, err)
		}
	}

	// With the orphan gone, the dropped volume becomes removable.
	got = list()
	dropped = expect(got, LeftoverDroppedVolume, alive+"_cache", true)
	for _, l := range []Leftover{dropped, ownerless, anon} {
		freed, err := e.dc.RemoveLeftover(l, scope)
		if err != nil {
			t.Fatalf("remove %s %s: %v", l.Kind, l.Name, err)
		}
		if l.SizeKnown && freed != l.SizeMB {
			t.Fatalf("%s freed %.2f MB, want measured %.2f MB", l.Name, freed, l.SizeMB)
		}
	}

	// A stale entry is refused, not retried against something else.
	if _, err := e.dc.RemoveLeftover(anon, scope); !errors.Is(err, ErrLeftoverChanged) {
		t.Fatalf("removing twice: err = %v, want ErrLeftoverChanged", err)
	}

	if got = list(); len(got) != 0 {
		t.Fatalf("leftovers remain after removal: %+v", got)
	}
	for _, v := range []string{alive + "_cache", gone + "_dbdata", anonVolume} {
		if _, err := e.docker("volume", "inspect", v); err == nil {
			t.Fatalf("volume %s still exists", v)
		}
	}
	// The current project's own volume and service are untouched.
	if _, err := e.docker("volume", "inspect", alive+"_data"); err != nil {
		t.Fatalf("current project volume was removed: %v", err)
	}
	if out := e.mustDocker("inspect", "-f", "{{.State.Status}}", alive+"-web-1"); out != "running" {
		t.Fatalf("current project service state = %q, want running", out)
	}
}
