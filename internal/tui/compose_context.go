package tui

import (
	"strings"

	composectx "github.com/0206pdh/dockviz-cli/internal/compose"
	"github.com/0206pdh/dockviz-cli/internal/docker"
)

func (m Model) composeServiceForContainer(c docker.ContainerInfo) (composectx.ServiceContext, bool) {
	if m.composeContext == nil || c.ComposeService == "" {
		return composectx.ServiceContext{}, false
	}
	service, ok := m.composeContext.Services[c.ComposeService]
	return service, ok
}

func (m Model) composeServiceByContainerName(name string) (composectx.ServiceContext, bool) {
	c, ok := m.containerByName(name)
	if !ok {
		return composectx.ServiceContext{}, false
	}
	return m.composeServiceForContainer(c)
}

// leftoverScope turns the loaded compose file into the scope leftover
// detection compares against, or nil when no compose file is loaded.
func (m Model) leftoverScope() *docker.ComposeScope {
	if m.composeContext == nil {
		return nil
	}
	scope := &docker.ComposeScope{
		Project:  m.composeContext.ProjectName,
		Services: make(map[string]bool, len(m.composeContext.Services)),
		Volumes:  make(map[string]bool, len(m.composeContext.Volumes)),
	}
	for name := range m.composeContext.Services {
		scope.Services[name] = true
	}
	for _, v := range m.composeContext.Volumes {
		scope.Volumes[v] = true
	}
	return scope
}

func (m Model) composeFilesLabel() string {
	if m.composeContext == nil || len(m.composeContext.Files) == 0 {
		return ""
	}
	files := make([]string, 0, len(m.composeContext.Files))
	for _, file := range m.composeContext.Files {
		files = append(files, shortPath(file))
	}
	return strings.Join(files, ", ")
}

func joinOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ", ")
}

func shortPath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(path, "/")
	if len(parts) <= 2 {
		return path
	}
	return strings.Join(parts[len(parts)-2:], "/")
}
