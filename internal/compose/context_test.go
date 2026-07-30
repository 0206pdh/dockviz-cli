package compose

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadComposeContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	content := []byte(`
name: shop
services:
  api:
    image: my-api:latest
    depends_on:
      - db
    networks:
      - backend
    volumes:
      - data:/data:ro
    ports:
      - "8080:80"
    cpus: 0.75
    mem_limit: 512m
    restart: unless-stopped
  db:
    image: postgres:16
    networks:
      - backend
volumes:
  data:
networks:
  backend:
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, err := Load(context.Background(), dir, nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if ctx == nil {
		t.Fatal("Load() returned nil context")
	}
	if ctx.ProjectName != "shop" {
		t.Fatalf("ProjectName = %q, want shop", ctx.ProjectName)
	}
	api := ctx.Services["api"]
	if api.Name != "api" || api.Image != "my-api:latest" {
		t.Fatalf("api context = %+v", api)
	}
	if !contains(api.DependsOn, "db") {
		t.Fatalf("api dependencies = %+v, want db", api.DependsOn)
	}
	if !contains(ctx.Services["db"].Dependents, "api") {
		t.Fatalf("db dependents = %+v, want api", ctx.Services["db"].Dependents)
	}
	if !contains(api.Networks, "backend") || !contains(api.Volumes, "data -> /data (ro)") {
		t.Fatalf("api networks/volumes = %+v / %+v", api.Networks, api.Volumes)
	}
	if api.CPUs != 0.75 || api.MemoryLimitMB != 512 {
		t.Fatalf("api limits = CPU %v MEM %v, want 0.75 and 512", api.CPUs, api.MemoryLimitMB)
	}
}

func TestLoadComposeContextMissingExplicitFile(t *testing.T) {
	_, err := Load(context.Background(), t.TempDir(), []string{"missing.yaml"})
	if err == nil {
		t.Fatal("Load() error = nil, want missing file error")
	}
}

func TestLoadComposeContextNoDiscoveredFile(t *testing.T) {
	ctx, err := Load(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if ctx != nil {
		t.Fatalf("Load() = %+v, want nil when no compose file is present", ctx)
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
