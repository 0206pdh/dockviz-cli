package docker

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/docker/docker/api/types/image"
)

func TestIsDangling(t *testing.T) {
	tests := []struct {
		name string
		img  *image.Summary
		want bool
	}{
		{"no tags", &image.Summary{RepoTags: nil}, true},
		{"single none tag", &image.Summary{RepoTags: []string{"<none>:<none>"}}, true},
		{"tagged image", &image.Summary{RepoTags: []string{"nginx:latest"}}, false},
		{"multiple tags", &image.Summary{RepoTags: []string{"nginx:latest", "nginx:1.25"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDangling(tt.img); got != tt.want {
				t.Errorf("isDangling() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBytesToMB(t *testing.T) {
	tests := []struct {
		name string
		b    int64
		want float64
	}{
		{"zero", 0, 0},
		{"one MB", 1024 * 1024, 1},
		{"half MB", 512 * 1024, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bytesToMB(tt.b); got != tt.want {
				t.Errorf("bytesToMB(%d) = %v, want %v", tt.b, got, tt.want)
			}
		})
	}
}

func TestLocalDockerHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"", true},
		{"npipe:////./pipe/docker_engine", true},
		{"unix:///var/run/docker.sock", true},
		{"tcp://localhost:2375", true},
		{"tcp://127.0.0.1:2375", true},
		{"tcp://192.168.1.100:2375", false},
		{"ssh://docker@example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := localDockerHost(tt.host); got != tt.want {
				t.Errorf("localDockerHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestDockerDesktopVHDXPaths(t *testing.T) {
	paths := dockerDesktopVHDXPaths(`C:\Users\me\AppData\Local`, `C:\Users\me`)
	want := `C:\Users\me\AppData\Local\Docker\wsl\disk\docker_data.vhdx`
	if len(paths) == 0 || paths[0] != want {
		t.Fatalf("dockerDesktopVHDXPaths()[0] = %q, want %q", paths[0], want)
	}
}

func TestFormatSizeZeroAndSmallValues(t *testing.T) {
	tests := []struct {
		name string
		mb   float64
		want string
	}{
		{"zero is explicit", 0, "0B"},
		{"small value is not rounded away", 0.5, "512 KB"},
		{"megabyte value", 1, "1 MB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatSize(tt.mb); got != tt.want {
				t.Errorf("FormatSize(%v) = %q, want %q", tt.mb, got, tt.want)
			}
		})
	}
}

func TestLogFileSizes(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		active, rotated, denied := logFileSizes("")
		if active != 0 || len(rotated) != 0 || denied {
			t.Errorf("logFileSizes(\"\") = (%v, %v, %v), want (0, empty, false)", active, rotated, denied)
		}
	})

	t.Run("active file plus rotated siblings", func(t *testing.T) {
		dir := t.TempDir()
		logPath := filepath.Join(dir, "abc123-json.log")

		write := func(path string, size int) {
			if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
				t.Fatalf("WriteFile(%s) error = %v", path, err)
			}
		}
		write(logPath, 2*1024*1024)                           // active: 2MB
		write(logPath+".1", 1024*1024)                        // rotated: 1MB
		write(logPath+".2.gz", 512*1024)                      // rotated (compressed): 0.5MB
		write(filepath.Join(dir, "other.log"), 999*1024*1024) // unrelated file, must be ignored

		activeMB, rotatedMB, denied := logFileSizes(logPath)
		if activeMB != 2 {
			t.Errorf("activeMB = %v, want 2", activeMB)
		}
		if denied {
			t.Error("permDenied = true, want false")
		}
		if len(rotatedMB) != 2 {
			t.Fatalf("len(rotatedMB) = %d, want 2: %v", len(rotatedMB), rotatedMB)
		}
		if rotatedMB[logPath+".1"] != 1 {
			t.Errorf("rotatedMB[%s.1] = %v, want 1", logPath, rotatedMB[logPath+".1"])
		}
		if rotatedMB[logPath+".2.gz"] != 0.5 {
			t.Errorf("rotatedMB[%s.2.gz] = %v, want 0.5", logPath, rotatedMB[logPath+".2.gz"])
		}
	})

	t.Run("permission denied", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows ACLs don't produce a POSIX-style permission error via os.Chmod")
		}
		if os.Geteuid() == 0 {
			t.Skip("running as root bypasses filesystem permission checks")
		}

		dir := t.TempDir()
		logPath := filepath.Join(dir, "abc123-json.log")
		if err := os.WriteFile(logPath, []byte("secret"), 0o644); err != nil {
			t.Fatalf("WriteFile error = %v", err)
		}
		if err := os.Chmod(dir, 0o000); err != nil {
			t.Fatalf("Chmod error = %v", err)
		}
		defer os.Chmod(dir, 0o755) // restore so t.TempDir() cleanup can remove it

		_, _, denied := logFileSizes(logPath)
		if !denied {
			t.Error("permDenied = false, want true when the containing directory isn't traversable")
		}
	})

	t.Run("nonexistent log path", func(t *testing.T) {
		active, rotated, denied := logFileSizes(filepath.Join(t.TempDir(), "missing-json.log"))
		if active != 0 || len(rotated) != 0 || !denied {
			t.Errorf("logFileSizes(missing) = (%v, %v, %v), want (0, empty, true)", active, rotated, denied)
		}
	})
}

func TestDemoClientPruneShrinksDiskUsage(t *testing.T) {
	d := NewDemoClient()

	before, err := d.DiskUsage()
	if err != nil {
		t.Fatalf("DiskUsage() error = %v", err)
	}
	if before.Images.ReclaimMB == 0 {
		t.Fatal("expected non-zero reclaimable images before pruning")
	}

	if _, err := d.PruneImages(); err != nil {
		t.Fatalf("PruneImages() error = %v", err)
	}

	after, err := d.DiskUsage()
	if err != nil {
		t.Fatalf("DiskUsage() error = %v", err)
	}
	if after.Images.ReclaimMB != 0 {
		t.Errorf("Images.ReclaimMB after prune = %v, want 0", after.Images.ReclaimMB)
	}
	// Pruning images must not affect other categories.
	if after.BuildCache.ReclaimMB != before.BuildCache.ReclaimMB {
		t.Errorf("BuildCache.ReclaimMB changed after pruning images: before=%v after=%v",
			before.BuildCache.ReclaimMB, after.BuildCache.ReclaimMB)
	}
}
