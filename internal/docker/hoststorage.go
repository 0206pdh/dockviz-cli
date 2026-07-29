package docker

import (
	"net"
	"net/url"
	"path/filepath"
	"strings"
)

// HostStorageInfo describes storage owned by the local Docker runtime outside
// Docker's daemon-level system/df accounting. On Windows Docker Desktop WSL2
// this is the docker_data.vhdx file that can stay expanded after docker prune.
type HostStorageInfo struct {
	Label       string
	Kind        string
	Path        string
	AllocatedMB float64
	HostFreeMB  float64
	Available   bool
	Unavailable string
}

func dockerDesktopVHDXPaths(localAppData, userProfile string) []string {
	var paths []string
	if localAppData != "" {
		paths = append(paths,
			filepath.Join(localAppData, "Docker", "wsl", "disk", "docker_data.vhdx"),
			filepath.Join(localAppData, "Docker", "wsl", "data", "ext4.vhdx"),
		)
	}
	if userProfile != "" {
		paths = append(paths,
			filepath.Join(userProfile, "AppData", "Local", "Docker", "wsl", "disk", "docker_data.vhdx"),
			filepath.Join(userProfile, "AppData", "Local", "Docker", "wsl", "data", "ext4.vhdx"),
		)
	}
	return paths
}

func localDockerHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return true
	}
	if strings.HasPrefix(host, "npipe://") || strings.HasPrefix(host, "unix://") {
		return true
	}

	u, err := url.Parse(host)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "tcp", "http", "https":
		h := u.Hostname()
		if h == "" {
			return false
		}
		if h == "localhost" {
			return true
		}
		ip := net.ParseIP(h)
		return ip != nil && ip.IsLoopback()
	default:
		return false
	}
}
