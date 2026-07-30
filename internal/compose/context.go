// Package compose loads Docker Compose files and exposes the service context
// dockviz needs to explain runtime problems.
package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/compose-spec/compose-go/v2/loader"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

var defaultComposeFiles = []string{
	"compose.yaml",
	"compose.yml",
	"docker-compose.yaml",
	"docker-compose.yml",
}

// Context is the compose information dockviz overlays on top of live Docker
// daemon data.
type Context struct {
	ProjectName string
	WorkingDir  string
	Files       []string
	Services    map[string]ServiceContext
}

// ServiceContext describes the service-level blast radius of a running
// container: dependencies, dependents, networks, volumes, and configured limits.
type ServiceContext struct {
	Name          string
	Image         string
	DependsOn     []string
	Dependents    []string
	Networks      []string
	Volumes       []string
	Ports         []string
	CPUs          float64
	MemoryLimitMB float64
	Restart       string
}

// Load discovers and parses compose files. Empty paths means auto-discover
// from workingDir. Missing files are not fatal: dockviz can still run from
// daemon labels only.
func Load(ctx context.Context, workingDir string, paths []string) (*Context, error) {
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	workingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return nil, err
	}

	files, err := resolveComposeFiles(workingDir, paths)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}

	details := composeTypes.ConfigDetails{
		WorkingDir:  workingDir,
		ConfigFiles: composeTypes.ToConfigFiles(files),
		Environment: composeTypes.NewMapping(os.Environ()),
	}
	project, err := loader.LoadWithContext(ctx, details)
	if err != nil {
		return nil, err
	}

	out := &Context{
		ProjectName: project.Name,
		WorkingDir:  workingDir,
		Files:       files,
		Services:    map[string]ServiceContext{},
	}

	for _, service := range project.Services {
		sc := serviceContext(project, service)
		out.Services[sc.Name] = sc
	}
	return out, nil
}

func resolveComposeFiles(workingDir string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		var found []string
		for _, name := range defaultComposeFiles {
			path := filepath.Join(workingDir, name)
			if fileExists(path) {
				found = append(found, path)
			}
		}
		return found, nil
	}

	files := make([]string, 0, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		path = filepath.Clean(path)
		if !fileExists(path) {
			return nil, fmt.Errorf("compose file not found: %s", path)
		}
		files = append(files, path)
	}
	return files, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func serviceContext(project *composeTypes.Project, service composeTypes.ServiceConfig) ServiceContext {
	dependsOn := service.GetDependencies()
	dependents := service.GetDependents(project)
	networks := mapKeys(service.Networks)
	volumes := serviceVolumes(service.Volumes)
	ports := servicePorts(service.Ports)

	return ServiceContext{
		Name:          service.Name,
		Image:         service.Image,
		DependsOn:     dependsOn,
		Dependents:    dependents,
		Networks:      networks,
		Volumes:       volumes,
		Ports:         ports,
		CPUs:          float64(service.CPUS),
		MemoryLimitMB: float64(service.MemLimit) / 1024 / 1024,
		Restart:       service.Restart,
	}
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func serviceVolumes(volumes []composeTypes.ServiceVolumeConfig) []string {
	out := make([]string, 0, len(volumes))
	for _, v := range volumes {
		entry := v.Target
		if v.Source != "" {
			entry = v.Source + " -> " + v.Target
		}
		if v.ReadOnly {
			entry += " (ro)"
		}
		out = append(out, entry)
	}
	sort.Strings(out)
	return out
}

func servicePorts(ports []composeTypes.ServicePortConfig) []string {
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		target := fmt.Sprintf("%d/%s", p.Target, p.Protocol)
		if p.Published != "" {
			target = p.Published + "->" + target
		}
		out = append(out, target)
	}
	sort.Strings(out)
	return out
}
