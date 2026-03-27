// demo.go provides fake Docker data for running dockviz without a live Docker daemon.
// Used with the --demo flag for portfolio demonstrations and CI previews.
package docker

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// DemoClient mimics Client but returns static/animated fake data.
type DemoClient struct {
	tick int
}

// NewDemoClient returns a demo client that needs no Docker daemon.
func NewDemoClient() *DemoClient {
	return &DemoClient{}
}

// Close is a no-op for demo mode.
func (d *DemoClient) Close() {}

// ListContainers returns a realistic set of fake containers.
func (d *DemoClient) ListContainers() ([]ContainerInfo, error) {
	d.tick++
	t := float64(d.tick)

	return []ContainerInfo{
		{
			ID:      "a1b2c3d4e5f6",
			Name:    "nginx-proxy",
			Image:   "nginx:1.25-alpine",
			Status:  "running",
			CPUPerc: wave(t, 2.1, 0.8, 0),
			MemMB:   wave(t, 45, 5, 1),
			Ports:   "80:80, 443:443",
		},
		{
			ID:      "b2c3d4e5f6a1",
			Name:    "api-server",
			Image:   "node:20-alpine",
			Status:  "running",
			CPUPerc: wave(t, 18.4, 6.0, 2),
			MemMB:   wave(t, 210, 30, 3),
			Ports:   "3000:3000",
		},
		{
			ID:      "c3d4e5f6a1b2",
			Name:    "postgres-db",
			Image:   "postgres:16",
			Status:  "running",
			CPUPerc: wave(t, 0.9, 0.3, 4),
			MemMB:   wave(t, 128, 10, 5),
			Ports:   "5432",
		},
		{
			ID:      "d4e5f6a1b2c3",
			Name:    "redis-cache",
			Image:   "redis:7-alpine",
			Status:  "running",
			CPUPerc: wave(t, 0.2, 0.1, 6),
			MemMB:   wave(t, 12, 2, 7),
			Ports:   "6379",
		},
		{
			ID:      "e5f6a1b2c3d4",
			Name:    "worker",
			Image:   "myapp/worker:latest",
			Status:  "running",
			CPUPerc: wave(t, 34.5, 12.0, 8),
			MemMB:   wave(t, 320, 40, 9),
			Ports:   "-",
		},
		{
			ID:     "f6a1b2c3d4e5",
			Name:   "db-migration",
			Image:  "myapp/migrate:v2.1.0",
			Status: "exited",
			Ports:  "-",
		},
	}, nil
}

// ListNetworks returns fake network topology data.
func (d *DemoClient) ListNetworks() ([]NetworkInfo, error) {
	return []NetworkInfo{
		{
			ID:         "net001aabbcc",
			Name:       "app-network",
			Driver:     "bridge",
			Containers: []string{"nginx-proxy", "api-server", "worker"},
		},
		{
			ID:         "net002bbccdd",
			Name:       "db-network",
			Driver:     "bridge",
			Containers: []string{"api-server", "postgres-db", "redis-cache"},
		},
		{
			ID:         "net003ccddef",
			Name:       "host",
			Driver:     "host",
			Containers: []string{},
		},
	}, nil
}

// ListImages returns fake image data.
func (d *DemoClient) ListImages() ([]ImageInfo, error) {
	return []ImageInfo{
		{ID: "sha256:a1b2c3d4", Tags: "nginx:1.25-alpine", SizeMB: 41},
		{ID: "sha256:b2c3d4e5", Tags: "node:20-alpine", SizeMB: 178},
		{ID: "sha256:c3d4e5f6", Tags: "postgres:16", SizeMB: 425},
		{ID: "sha256:d4e5f6a1", Tags: "redis:7-alpine", SizeMB: 34},
		{ID: "sha256:e5f6a1b2", Tags: "myapp/worker:latest", SizeMB: 312},
		{ID: "sha256:f6a1b2c3", Tags: "myapp/migrate:v2.1.0", SizeMB: 98},
	}, nil
}

// Ping always succeeds in demo mode.
func (d *DemoClient) Ping() error { return nil }

// StartContainer simulates a start (prints nothing, just delays).
func (d *DemoClient) StartContainer(id string) error {
	time.Sleep(300 * time.Millisecond)
	return nil
}

// StopContainer simulates a stop.
func (d *DemoClient) StopContainer(id string) error {
	time.Sleep(300 * time.Millisecond)
	return nil
}

// RestartContainer simulates a restart.
func (d *DemoClient) RestartContainer(id string) error {
	time.Sleep(500 * time.Millisecond)
	return nil
}

// FetchStats returns animated fake stats (not used — stats are inlined in ListContainers for demo).
func (d *DemoClient) FetchStats(id string) (float64, float64, error) {
	return rand.Float64() * 20, rand.Float64() * 200, nil
}

// RemoveContainer simulates a container removal with a short delay.
func (d *DemoClient) RemoveContainer(id string) error {
	time.Sleep(200 * time.Millisecond)
	return nil
}

// RemoveImage simulates an image removal.
func (d *DemoClient) RemoveImage(id string) error {
	time.Sleep(200 * time.Millisecond)
	return nil
}

// StreamLogs simulates a live log stream with pre-canned demo lines.
// Each line is emitted with a 500ms delay to mimic a real service.
func (d *DemoClient) StreamLogs(ctx context.Context, id string) <-chan LogLine {
	ch := make(chan LogLine, 10)
	go func() {
		defer close(ch)
		lines := []string{
			"2026-03-27T10:00:00Z [INFO] Server started on :3000",
			"2026-03-27T10:00:01Z [INFO] Connected to database",
			"2026-03-27T10:00:05Z [INFO] GET /api/health 200 OK",
			"2026-03-27T10:00:10Z [INFO] GET /api/users 200 OK",
			"2026-03-27T10:00:15Z [WARN] Rate limit approaching",
		}
		for _, l := range lines {
			select {
			case ch <- LogLine{Text: l}:
			case <-ctx.Done():
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()
	return ch
}

// wave produces a smoothly oscillating value for realistic-looking animated stats.
func wave(t, base, amp float64, phase int) float64 {
	v := base + amp*math.Sin(t*0.3+float64(phase))
	return math.Max(0, v)
}

// Fmt helper used internally
var _ = fmt.Sprintf
