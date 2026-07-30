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
