//go:build !windows

package docker

func detectDockerDesktopHostStorage(host string) HostStorageInfo {
	return HostStorageInfo{
		Label:       "Docker Desktop VHDX",
		Kind:        "Docker Desktop WSL2 virtual disk",
		Unavailable: "Docker Desktop VHDX measurement is only available on Windows Docker Desktop WSL2",
	}
}
