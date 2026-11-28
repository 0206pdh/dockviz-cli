// Command leftovers is a developer validation scenario for the leftover
// resource detection plan (docs/leftovers-plan.ko.md, Phase 1 / T-024).
//
// It deliberately creates the kinds of Docker resources that tend to be left
// behind — stopped containers, dangling images, anonymous volumes, Compose
// orphans and one-off containers, volumes of a project that was brought down,
// a separate buildx builder cache, user networks — then records what the
// Docker API reports for each of them, and finally removes everything it made.
//
// Resources are created through the docker CLI, the way a user would, and
// inspected through the Go SDK, the way dockviz does. Everything carries the
// label dockviz.test=leftovers; resources Docker creates without labels
// (anonymous volumes, the buildx state volume) are tracked by name.
//
//	go run ./scenarios/leftovers                  # create, record, clean up
//	go run ./scenarios/leftovers --keep           # leave resources for dockviz
//	go run ./scenarios/leftovers --cleanup        # remove a previous --keep run
//
// Run it against a disposable or otherwise idle daemon.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	dockerd "github.com/0206pdh/dockviz-cli/internal/docker"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

const (
	testLabel    = "dockviz.test"
	testValue    = "leftovers"
	baseImage    = "busybox:1.37"
	projectAlive = "dvzleft"
	projectGone  = "dvzgone"
	builderName  = "dvz-leftovers"
	appImage     = "dvz-leftovers/app:latest"
	anonImage    = "dvz-leftovers/anon:latest"
	stoppedName  = "dvz-leftovers-stopped"
	anonName     = "dvz-leftovers-anon"
	networkCount = 3
)

var labelFilter = testLabel + "=" + testValue

// Report is the JSON written to --out. Snapshot holds raw SDK responses so
// Phase 2 can reuse them as test fixtures.
type Report struct {
	GeneratedAt time.Time      `json:"generatedAt"`
	Host        string         `json:"host"`
	Endpoint    string         `json:"endpoint"`
	Environment map[string]any `json:"environment"`
	Steps       []Step         `json:"steps"`
	Checks      []Check        `json:"checks"`
	Snapshot    Snapshot       `json:"snapshot"`
}

type Step struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Output string `json:"output,omitempty"`
}

// Check answers one question from the plan with evidence from the snapshot.
type Check struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Evidence string `json:"evidence,omitempty"`
}

type Snapshot struct {
	Containers []container.Summary         `json:"containers"`
	Inspects   []container.InspectResponse `json:"inspects"`
	Volumes    []*volume.Volume            `json:"volumes"`
	Images     []image.Summary             `json:"images"`
	// NewImages is every image (no label filter) that did not exist before
	// the run, and DanglingNew the subset the daemon's dangling=true filter
	// returns. They show whether a replaced tag survives under a different
	// shape than the label filter can see (containerd image store).
	NewImages     []image.Summary     `json:"newImages"`
	DanglingNew   []image.Summary     `json:"danglingNew"`
	Networks      []network.Summary   `json:"networks"`
	AllNetworks   int                 `json:"allNetworks"`
	DiskUsage     DiskUsageView       `json:"diskUsage"`
	BuildCacheRaw []*types.BuildCache `json:"buildCache"`
}

// DiskUsageView keeps only this scenario's objects from /system/df.
type DiskUsageView struct {
	Containers []*container.Summary `json:"containers"`
	Volumes    []*volume.Volume     `json:"volumes"`
	Images     []*image.Summary     `json:"images"`
}

type runner struct {
	ctx        context.Context
	cli        *client.Client
	ep         dockerd.Endpoint
	work       string
	report     *Report
	anonVols   map[string]bool
	preImages  map[string]bool
	hasCompose bool
	hasBuildx  bool
}

func main() {
	keep := flag.Bool("keep", false, "leave the created resources in place (inspect them with dockviz, then run --cleanup)")
	cleanupOnly := flag.Bool("cleanup", false, "only remove resources left by a previous --keep run")
	out := flag.String("out", "", "report path (default: leftovers-<timestamp>.json in the current directory)")
	host := flag.String("host", "", "Docker host, same as dockviz --host")
	dctx := flag.String("context", "", "Docker context, same as dockviz --context")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cli, ep, err := dockerd.NewSDKClient(*host, *dctx)
	if err != nil {
		fatalf("%v", err)
	}
	defer cli.Close()
	if _, err := cli.Ping(ctx); err != nil {
		fatalf("cannot reach Docker daemon at %s (from %s): %v", cli.DaemonHost(), ep.Source, err)
	}

	work, err := os.MkdirTemp("", "dvz-leftovers-")
	if err != nil {
		fatalf("%v", err)
	}
	defer os.RemoveAll(work)

	r := &runner{
		ctx:       ctx,
		cli:       cli,
		ep:        ep,
		work:      work,
		report:    &Report{GeneratedAt: time.Now().UTC(), Host: cli.DaemonHost(), Endpoint: ep.Source},
		anonVols:  map[string]bool{},
		preImages: map[string]bool{},
	}
	r.hasCompose = r.cliWorks("compose", "version")
	r.hasBuildx = r.cliWorks("buildx", "version")
	if err := r.writeProjectFiles(); err != nil {
		fatalf("%v", err)
	}

	if *cleanupOnly {
		r.loadState()
		r.cleanup()
		r.printSteps()
		if !r.failed() {
			_ = os.Remove(statePath())
		}
		return
	}

	r.recordEnvironment()
	r.recordPreexistingImages()
	r.createAll()
	r.snapshot()
	r.check()

	path := *out
	if path == "" {
		path = fmt.Sprintf("leftovers-%s.json", time.Now().Format("20060102-150405"))
	}
	if err := writeJSON(path, r.report); err != nil {
		fatalf("write report: %v", err)
	}

	if *keep {
		if err := r.saveState(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save state for --cleanup: %v\n", err)
		}
		fmt.Println("\n--keep: resources left in place. Remove them with: go run ./scenarios/leftovers --cleanup")
	} else {
		r.cleanup()
	}
	r.printSteps()
	r.printChecks()
	fmt.Printf("\nreport: %s\n", path)
}

// ---------------------------------------------------------------------------
// creation

func (r *runner) createAll() {
	r.step("pull "+baseImage, func() (string, error) { return "", r.pull(baseImage) })

	// 1. A stopped container: docker run without --rm.
	r.step("stopped container (docker run without --rm)", func() (string, error) {
		return r.docker("run", "--name", stoppedName, "--label", labelFilter, baseImage, "sh", "-c", "echo bye")
	})

	// 2. Dangling image: build the same tag twice with different content.
	for i := 1; i <= 2; i++ {
		i := i
		r.step(fmt.Sprintf("build %s #%d (same tag)", appImage, i), func() (string, error) {
			return r.docker("build", "-q", "--build-arg", fmt.Sprintf("REV=%d", i), "-t", appImage, "-f", "app.Dockerfile", ".")
		})
	}

	// 3. Anonymous volume: image with VOLUME, container removed without -v.
	r.step("build "+anonImage+" (VOLUME /data)", func() (string, error) {
		return r.docker("build", "-q", "-t", anonImage, "-f", "anon.Dockerfile", ".")
	})
	r.step("anonymous volume (docker rm without -v)", func() (string, error) {
		if out, err := r.docker("run", "--name", anonName, "--label", labelFilter, anonImage); err != nil {
			return out, err
		}
		if err := r.trackAnonVolumes(anonName); err != nil {
			return "", err
		}
		return r.docker("rm", anonName)
	})

	// 4. Compose: orphan service, one-off container, dropped volume.
	if r.hasCompose {
		r.step("compose up "+projectAlive+" (v1: web, worker, volumes data+cache)", func() (string, error) {
			return r.docker("compose", "-p", projectAlive, "-f", "alive-v1.yaml", "up", "-d", "--wait")
		})
		r.step("compose run without --rm (one-off)", func() (string, error) {
			return r.docker("compose", "-p", projectAlive, "-f", "alive-v1.yaml", "run", "web", "echo", "one-off")
		})
		r.step("compose up "+projectAlive+" (v2: worker and cache removed, no --remove-orphans)", func() (string, error) {
			return r.docker("compose", "-p", projectAlive, "-f", "alive-v2.yaml", "up", "-d", "--wait")
		})

		// 5. A project brought down without -v.
		r.step("compose up "+projectGone, func() (string, error) {
			return r.docker("compose", "-p", projectGone, "-f", "gone.yaml", "up", "-d", "--wait")
		})
		r.step("compose down "+projectGone+" (without -v)", func() (string, error) {
			return r.docker("compose", "-p", projectGone, "-f", "gone.yaml", "down")
		})
	} else {
		r.skip("compose scenarios", "`docker compose` is not available")
	}

	// 6. A separate buildx builder with its own cache.
	if r.hasBuildx {
		r.step("buildx create "+builderName+" (docker-container driver)", func() (string, error) {
			return r.docker("buildx", "create", "--name", builderName, "--driver", "docker-container", "--bootstrap")
		})
		r.step("buildx build on "+builderName+" (cache only, no --load)", func() (string, error) {
			return r.docker("buildx", "build", "--builder", builderName, "--build-arg", "REV=buildx", "-f", "app.Dockerfile", ".")
		})
	} else {
		r.skip("buildx scenarios", "`docker buildx` is not available")
	}

	// 7. User networks.
	for i := 1; i <= networkCount; i++ {
		name := fmt.Sprintf("dvz-leftovers-net%d", i)
		r.step("network create "+name, func() (string, error) {
			return r.docker("network", "create", "--label", labelFilter, name)
		})
	}

	// Let the compose services finish writing into their volumes.
	time.Sleep(3 * time.Second)
}

func (r *runner) writeProjectFiles() error {
	files := map[string]string{
		"app.Dockerfile": `FROM ` + baseImage + `
ARG REV=0
LABEL ` + testLabel + `=` + testValue + `
RUN echo "rev $REV" > /rev && dd if=/dev/urandom of=/blob bs=1M count=2 2>/dev/null
`,
		"anon.Dockerfile": `FROM ` + baseImage + `
LABEL ` + testLabel + `=` + testValue + `
VOLUME /data
CMD ["sh", "-c", "dd if=/dev/zero of=/data/f bs=1M count=3 2>/dev/null"]
`,
		"alive-v1.yaml": composeFile([]string{"web", "worker"}, []string{"data", "cache"}),
		"alive-v2.yaml": composeFile([]string{"web"}, []string{"data"}),
		"gone.yaml":     composeFile([]string{"db"}, []string{"dbdata"}),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(r.work, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// composeFile renders services that each mount the first volume (and the
// service named "worker" also mounts "cache") and write a few MB into it.
func composeFile(services, volumes []string) string {
	var b strings.Builder
	b.WriteString("services:\n")
	for _, s := range services {
		vol := volumes[0]
		if s == "worker" && len(volumes) > 1 {
			vol = volumes[1]
		}
		fmt.Fprintf(&b, `  %s:
    image: %s
    labels:
      %s: %s
    command: ["sh", "-c", "dd if=/dev/zero of=/data/%s bs=1M count=4 2>/dev/null; sleep 3600"]
    volumes:
      - %s:/data
`, s, baseImage, testLabel, testValue, s, vol)
	}
	b.WriteString("volumes:\n")
	for _, v := range volumes {
		fmt.Fprintf(&b, "  %s:\n    labels:\n      %s: %s\n", v, testLabel, testValue)
	}
	fmt.Fprintf(&b, "networks:\n  default:\n    labels:\n      %s: %s\n", testLabel, testValue)
	return b.String()
}

func (r *runner) trackAnonVolumes(containerName string) error {
	insp, err := r.cli.ContainerInspect(r.ctx, containerName)
	if err != nil {
		return err
	}
	for _, m := range insp.Mounts {
		if m.Type == "volume" {
			r.anonVols[m.Name] = true
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// recording

func (r *runner) recordEnvironment() {
	info, err := r.cli.Info(r.ctx)
	if err != nil {
		r.report.Environment = map[string]any{"error": err.Error()}
		return
	}
	r.report.Environment = map[string]any{
		"serverVersion":   info.ServerVersion,
		"operatingSystem": info.OperatingSystem,
		"osType":          info.OSType,
		"architecture":    info.Architecture,
		"storageDriver":   info.Driver,
		"driverStatus":    info.DriverStatus,
		"loggingDriver":   info.LoggingDriver,
		"dockerRootDir":   info.DockerRootDir,
		"cgroupDriver":    info.CgroupDriver,
		"clientOS":        runtime.GOOS,
		"compose":         r.hasCompose,
		"buildx":          r.hasBuildx,
	}
}

func (r *runner) recordPreexistingImages() {
	imgs, err := r.cli.ImageList(r.ctx, image.ListOptions{All: true})
	if err != nil {
		return
	}
	for _, img := range imgs {
		r.preImages[img.ID] = true
	}
}

func (r *runner) snapshot() {
	s := &r.report.Snapshot
	lf := filters.NewArgs(filters.Arg("label", labelFilter))

	if cs, err := r.cli.ContainerList(r.ctx, container.ListOptions{All: true, Filters: lf}); err == nil {
		s.Containers = cs
		for _, c := range cs {
			if insp, err := r.cli.ContainerInspect(r.ctx, c.ID); err == nil {
				s.Inspects = append(s.Inspects, insp)
			}
		}
	}

	if vl, err := r.cli.VolumeList(r.ctx, volume.ListOptions{}); err == nil {
		for _, v := range vl.Volumes {
			if r.ours(v) {
				s.Volumes = append(s.Volumes, v)
			}
		}
	}

	// Dangling images lose their tag but keep the Dockerfile LABEL.
	if imgs, err := r.cli.ImageList(r.ctx, image.ListOptions{All: true, Filters: lf}); err == nil {
		s.Images = imgs
	}

	if imgs, err := r.cli.ImageList(r.ctx, image.ListOptions{All: true}); err == nil {
		for _, img := range imgs {
			if !r.preImages[img.ID] {
				s.NewImages = append(s.NewImages, img)
			}
		}
	}
	if imgs, err := r.cli.ImageList(r.ctx, image.ListOptions{All: true, Filters: filters.NewArgs(filters.Arg("dangling", "true"))}); err == nil {
		for _, img := range imgs {
			if !r.preImages[img.ID] {
				s.DanglingNew = append(s.DanglingNew, img)
			}
		}
	}

	if nets, err := r.cli.NetworkList(r.ctx, network.ListOptions{}); err == nil {
		s.AllNetworks = len(nets)
		for _, n := range nets {
			if n.Labels[testLabel] == testValue {
				s.Networks = append(s.Networks, n)
			}
		}
	}

	if du, err := r.cli.DiskUsage(r.ctx, types.DiskUsageOptions{}); err == nil {
		for _, c := range du.Containers {
			if c.Labels[testLabel] == testValue {
				s.DiskUsage.Containers = append(s.DiskUsage.Containers, c)
			}
		}
		for _, v := range du.Volumes {
			if r.ours(v) {
				s.DiskUsage.Volumes = append(s.DiskUsage.Volumes, v)
			}
		}
		for _, img := range du.Images {
			if img.Labels[testLabel] == testValue {
				s.DiskUsage.Images = append(s.DiskUsage.Images, img)
			}
		}
		s.BuildCacheRaw = du.BuildCache
	}
}

func (r *runner) ours(v *volume.Volume) bool {
	return v.Labels[testLabel] == testValue || r.anonVols[v.Name] || strings.HasPrefix(v.Name, "buildx_buildkit_"+builderName)
}

// ---------------------------------------------------------------------------
// checks: one per open question in docs/leftovers-plan.ko.md

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (r *runner) check() {
	s := r.report.Snapshot
	add := func(id, q, a, ev string) {
		r.report.Checks = append(r.report.Checks, Check{ID: id, Question: q, Answer: a, Evidence: ev})
	}
	byService := func(project, service string) []container.Summary {
		var out []container.Summary
		for _, c := range s.Containers {
			if c.Labels["com.docker.compose.project"] == project && c.Labels["com.docker.compose.service"] == service {
				out = append(out, c)
			}
		}
		return out
	}
	volByName := func(name string) *volume.Volume {
		for _, v := range s.DiskUsage.Volumes {
			if v.Name == name {
				return v
			}
		}
		return nil
	}
	usage := func(v *volume.Volume) string {
		if v == nil || v.UsageData == nil {
			return "UsageData=nil"
		}
		return fmt.Sprintf("size=%s refCount=%d", human(v.UsageData.Size), v.UsageData.RefCount)
	}

	// Stopped container and its finish time.
	for _, c := range s.Containers {
		if strings.TrimPrefix(firstName(c.Names), "/") != stoppedName {
			continue
		}
		finished := "(inspect missing)"
		for _, in := range s.Inspects {
			if in.ID == c.ID && in.State != nil {
				finished = in.State.FinishedAt
			}
		}
		add("stopped.finishedAt", "Is the stop time available from the list API, or only from inspect?",
			"list: State/Status text only; inspect: State.FinishedAt",
			fmt.Sprintf("list State=%q Status=%q; inspect FinishedAt=%s", c.State, c.Status, finished))
	}

	// Dangling image after rebuilding the same tag.
	var dangling, tagged []string
	for _, img := range s.Images {
		if len(img.RepoTags) == 0 || (len(img.RepoTags) == 1 && img.RepoTags[0] == "<none>:<none>") {
			dangling = append(dangling, fmt.Sprintf("%s tags=%v size=%s containers=%d", short(img.ID), img.RepoTags, human(img.Size), img.Containers))
		} else {
			tagged = append(tagged, fmt.Sprintf("%v", img.RepoTags))
		}
	}
	add("image.dangling", "Does rebuilding the same tag leave a dangling image that isDangling() recognises?",
		yesNo(len(dangling) > 0), fmt.Sprintf("label-filtered: dangling=%v tagged=%v", dangling, tagged))
	var newImgs, dfilter []string
	for _, img := range s.NewImages {
		newImgs = append(newImgs, fmt.Sprintf("%s tags=%v digests=%d size=%s label=%q", short(img.ID), img.RepoTags, len(img.RepoDigests), human(img.Size), img.Labels[testLabel]))
	}
	for _, img := range s.DanglingNew {
		dfilter = append(dfilter, short(img.ID))
	}
	add("image.newSinceStart", "Which images appeared during the run without any label filter, and which does dangling=true return?",
		fmt.Sprintf("%d new, %d dangling=true", len(s.NewImages), len(s.DanglingNew)),
		fmt.Sprintf("new=%v dangling=%v", newImgs, dfilter))
	var duImgs []string
	for _, img := range s.DiskUsage.Images {
		duImgs = append(duImgs, fmt.Sprintf("%s tags=%v size=%s", short(img.ID), img.RepoTags, human(img.Size)))
	}
	add("image.diskUsage", "How do this run's images appear in /system/df (what DiskUsage() reads)?",
		fmt.Sprintf("%d labelled images", len(s.DiskUsage.Images)), strings.Join(duImgs, "; "))

	// Anonymous volume.
	for name := range r.anonVols {
		v := volByName(name)
		labels := "volume missing"
		if v != nil {
			labels = fmt.Sprintf("labels=%v", v.Labels)
		}
		add("volume.anonymous", "How is an anonymous volume identifiable (64-hex name, com.docker.volume.anonymous label)?",
			fmt.Sprintf("64-hex name: %s; anonymous label: %s", yesNo(hex64.MatchString(name)), yesNo(v != nil && hasKey(v.Labels, "com.docker.volume.anonymous"))),
			fmt.Sprintf("%s %s %s", short(name), labels, usage(v)))
	}

	if r.hasCompose {
		orphans := byService(projectAlive, "worker")
		ev := "no worker container"
		if len(orphans) > 0 {
			ev = fmt.Sprintf("state=%s labels.oneoff=%q config_files=%q", orphans[0].State,
				orphans[0].Labels["com.docker.compose.oneoff"], orphans[0].Labels["com.docker.compose.project.config_files"])
		}
		add("compose.orphan", "Does a service removed from the file stay as a container, and is it still running?",
			yesNo(len(orphans) > 0), ev)

		var oneoff []string
		for _, c := range s.Containers {
			if c.Labels["com.docker.compose.project"] == projectAlive && strings.EqualFold(c.Labels["com.docker.compose.oneoff"], "true") {
				oneoff = append(oneoff, fmt.Sprintf("%s state=%s", firstName(c.Names), c.State))
			}
		}
		add("compose.oneoff", "Does `compose run` without --rm leave a container with com.docker.compose.oneoff=True?",
			yesNo(len(oneoff) > 0), strings.Join(oneoff, "; "))

		dropped := volByName(projectAlive + "_cache")
		add("compose.droppedVolume", "Does a volume removed from the file keep its project/volume labels?",
			yesNo(dropped != nil && dropped.Labels["com.docker.compose.project"] == projectAlive),
			volumeEvidence(dropped, usage))

		gone := volByName(projectGone + "_dbdata")
		add("compose.goneProjectVolume", "After `compose down` without -v, does the named volume remain with only labels to identify its project?",
			yesNo(gone != nil), volumeEvidence(gone, usage))

		var wd string
		for _, c := range s.Containers {
			if c.Labels["com.docker.compose.project"] == projectAlive {
				wd = c.Labels["com.docker.compose.project.working_dir"]
				break
			}
		}
		add("compose.workingDir", "Do volumes carry the project working_dir (containers do)?",
			yesNo(gone != nil && hasKey(gone.Labels, "com.docker.compose.project.working_dir")),
			fmt.Sprintf("container working_dir=%q", wd))
	}

	if r.hasBuildx {
		var states []string
		for _, v := range s.DiskUsage.Volumes {
			if strings.HasPrefix(v.Name, "buildx_buildkit_"+builderName) {
				states = append(states, v.Name+" "+usage(v))
			}
		}
		add("buildx.stateVolume", "Is a docker-container builder's cache a separate volume outside BuildCache?",
			yesNo(len(states) > 0), fmt.Sprintf("%v; daemon BuildCache records=%d", states, len(s.BuildCacheRaw)))
	}

	add("network.count", "How many networks exist (address pool pressure)?",
		fmt.Sprintf("%d total, %d created here", s.AllNetworks, len(s.Networks)), "")

	logs := map[string]int{}
	emptyLogPath := 0
	for _, in := range s.Inspects {
		if in.HostConfig != nil {
			logs[in.HostConfig.LogConfig.Type]++
		}
		if in.LogPath == "" {
			emptyLogPath++
		}
	}
	add("logs.driver", "Which log driver do containers use, and is LogPath empty?",
		fmt.Sprintf("drivers=%v, empty LogPath=%d/%d", logs, emptyLogPath, len(s.Inspects)),
		fmt.Sprintf("daemon default=%v", r.report.Environment["loggingDriver"]))
}

func volumeEvidence(v *volume.Volume, usage func(*volume.Volume) string) string {
	if v == nil {
		return "volume missing"
	}
	keys := make([]string, 0, len(v.Labels))
	for k := range v.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fmt.Sprintf("labels=%v createdAt=%s %s", keys, v.CreatedAt, usage(v))
}

// ---------------------------------------------------------------------------
// cleanup: label first, then names Docker created without labels

func (r *runner) cleanup() {
	if r.hasCompose {
		// v1 knows every service, so down --remove-orphans covers worker and one-offs.
		r.step("cleanup: compose down "+projectAlive, func() (string, error) {
			return r.docker("compose", "-p", projectAlive, "-f", "alive-v1.yaml", "down", "-v", "--remove-orphans")
		})
		r.step("cleanup: compose down "+projectGone, func() (string, error) {
			return r.docker("compose", "-p", projectGone, "-f", "gone.yaml", "down", "-v", "--remove-orphans")
		})
	}
	if r.hasBuildx {
		r.step("cleanup: buildx rm "+builderName, func() (string, error) {
			out, err := r.docker("buildx", "rm", "-f", builderName)
			if err != nil && strings.Contains(out, "no builder") {
				return out, nil
			}
			return out, err
		})
	}

	lf := filters.NewArgs(filters.Arg("label", labelFilter))
	r.step("cleanup: containers by label", func() (string, error) {
		cs, err := r.cli.ContainerList(r.ctx, container.ListOptions{All: true, Filters: lf})
		if err != nil {
			return "", err
		}
		var errs []error
		for _, c := range cs {
			// Record anonymous volumes before they lose their owner.
			for _, m := range c.Mounts {
				if m.Type == "volume" && hex64.MatchString(m.Name) {
					r.anonVols[m.Name] = true
				}
			}
			errs = append(errs, r.cli.ContainerRemove(r.ctx, c.ID, container.RemoveOptions{Force: true, RemoveVolumes: true}))
		}
		return fmt.Sprintf("%d removed", len(cs)), errors.Join(errs...)
	})

	r.step("cleanup: volumes", func() (string, error) {
		vl, err := r.cli.VolumeList(r.ctx, volume.ListOptions{})
		if err != nil {
			return "", err
		}
		var errs []error
		n := 0
		for _, v := range vl.Volumes {
			if r.ours(v) {
				errs = append(errs, r.cli.VolumeRemove(r.ctx, v.Name, false))
				n++
			}
		}
		return fmt.Sprintf("%d removed", n), errors.Join(errs...)
	})

	r.step("cleanup: networks by label", func() (string, error) {
		nets, err := r.cli.NetworkList(r.ctx, network.ListOptions{Filters: lf})
		if err != nil {
			return "", err
		}
		var errs []error
		for _, n := range nets {
			errs = append(errs, r.cli.NetworkRemove(r.ctx, n.ID))
		}
		return fmt.Sprintf("%d removed", len(nets)), errors.Join(errs...)
	})

	r.step("cleanup: images by label and pulled base images", func() (string, error) {
		imgs, err := r.cli.ImageList(r.ctx, image.ListOptions{All: true, Filters: lf})
		if err != nil {
			return "", err
		}
		var errs []error
		for _, img := range imgs {
			_, err := r.cli.ImageRemove(r.ctx, img.ID, image.RemoveOptions{Force: true, PruneChildren: true})
			errs = append(errs, err)
		}
		n := len(imgs)
		// Base images this run pulled; keep anything that existed before.
		for _, ref := range []string{baseImage, "moby/buildkit:buildx-stable-1"} {
			if r.preImages == nil {
				break // unknown what existed before; keep base images
			}
			insp, err := r.cli.ImageInspect(r.ctx, ref)
			if err != nil || r.preImages[insp.ID] {
				continue
			}
			if _, err := r.cli.ImageRemove(r.ctx, ref, image.RemoveOptions{PruneChildren: true}); err == nil {
				n++
			}
		}
		return fmt.Sprintf("%d removed", n), errors.Join(errs...)
	})
}

// ---------------------------------------------------------------------------
// --keep state: anonymous volumes carry no label, so their names are saved
// for a later --cleanup run.

type state struct {
	AnonVolumes []string `json:"anonVolumes"`
	PreImages   []string `json:"preImages"`
}

func statePath() string { return filepath.Join(os.TempDir(), "dvz-leftovers-state.json") }

func (r *runner) saveState() error {
	var st state
	for v := range r.anonVols {
		st.AnonVolumes = append(st.AnonVolumes, v)
	}
	for id := range r.preImages {
		st.PreImages = append(st.PreImages, id)
	}
	return writeJSON(statePath(), st)
}

func (r *runner) loadState() {
	data, err := os.ReadFile(statePath())
	if err != nil {
		// Without state, unlabelled anonymous volumes and pulled base images
		// are left alone rather than guessed at.
		r.preImages = nil
		return
	}
	var st state
	if json.Unmarshal(data, &st) != nil {
		r.preImages = nil
		return
	}
	for _, v := range st.AnonVolumes {
		r.anonVols[v] = true
	}
	for _, id := range st.PreImages {
		r.preImages[id] = true
	}
}

func (r *runner) failed() bool {
	for _, s := range r.report.Steps {
		if !s.OK {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// helpers

func (r *runner) step(name string, fn func() (string, error)) {
	if r.ctx.Err() != nil {
		r.report.Steps = append(r.report.Steps, Step{Name: name, Error: "interrupted"})
		return
	}
	fmt.Printf("• %s\n", name)
	out, err := fn()
	s := Step{Name: name, OK: err == nil, Output: tail(out, 600)}
	if err != nil {
		s.Error = err.Error()
		fmt.Printf("  ! %v\n", err)
	}
	r.report.Steps = append(r.report.Steps, s)
}

func (r *runner) skip(name, why string) {
	fmt.Printf("• %s (skipped: %s)\n", name, why)
	r.report.Steps = append(r.report.Steps, Step{Name: name, OK: true, Output: "skipped: " + why})
}

// docker runs the docker CLI against the same daemon the SDK client uses.
func (r *runner) docker(args ...string) (string, error) {
	cmd := exec.CommandContext(r.ctx, "docker", args...)
	cmd.Dir = r.work
	cmd.Env = os.Environ()
	if r.ep.Host != "" {
		cmd.Env = append(cmd.Env, "DOCKER_HOST="+r.ep.Host)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := strings.TrimSpace(buf.String())
	if err != nil {
		return out, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, tail(out, 300))
	}
	return out, nil
}

func (r *runner) cliWorks(args ...string) bool {
	_, err := r.docker(args...)
	return err == nil
}

func (r *runner) pull(ref string) error {
	rc, err := r.cli.ImagePull(r.ctx, ref, image.PullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(io.Discard, rc)
	return err
}

func (r *runner) printSteps() {
	failed := 0
	for _, s := range r.report.Steps {
		if !s.OK {
			failed++
		}
	}
	fmt.Printf("\nsteps: %d, failed: %d\n", len(r.report.Steps), failed)
	for _, s := range r.report.Steps {
		if !s.OK {
			fmt.Printf("  FAIL %s: %s\n", s.Name, s.Error)
		}
	}
}

func (r *runner) printChecks() {
	fmt.Println("\nchecks:")
	for _, c := range r.report.Checks {
		fmt.Printf("  [%s] %s\n      → %s\n", c.ID, c.Question, c.Answer)
		if c.Evidence != "" {
			fmt.Printf("        %s\n", c.Evidence)
		}
	}
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func hasKey(m map[string]string, k string) bool {
	_, ok := m[k]
	return ok
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func short(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func human(b int64) string {
	switch {
	case b < 0:
		return "unknown"
	case b >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%dB", b)
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "leftovers: "+format+"\n", args...)
	os.Exit(1)
}
