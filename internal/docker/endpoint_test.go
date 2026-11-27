package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/client"
)

// fakeDockerConfig builds a throwaway ~/.docker with an optional
// currentContext and a set of contexts (name -> docker endpoint host).
type fakeDockerConfig struct {
	t   *testing.T
	dir string
}

func newFakeDockerConfig(t *testing.T) *fakeDockerConfig {
	t.Helper()
	return &fakeDockerConfig{t: t, dir: t.TempDir()}
}

func (f *fakeDockerConfig) write(rel, content string) {
	f.t.Helper()
	p := filepath.Join(f.dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fakeDockerConfig) setCurrent(name string) {
	f.write("config.json", `{"auths":{},"currentContext":"`+name+`"}`)
}

func (f *fakeDockerConfig) addContext(name, host string, skipTLS bool) {
	skip := "false"
	if skipTLS {
		skip = "true"
	}
	f.write(filepath.Join("contexts", "meta", contextDirName(name), "meta.json"),
		`{"Name":"`+name+`","Metadata":{},"Endpoints":{"docker":{"Host":"`+host+`","SkipTLSVerify":`+skip+`}}}`)
}

// opts returns EndpointOptions that read only the fake config and env.
func (f *fakeDockerConfig) opts(env map[string]string) EndpointOptions {
	if env == nil {
		env = map[string]string{}
	}
	if _, ok := env["DOCKER_CONFIG"]; !ok {
		env["DOCKER_CONFIG"] = f.dir
	}
	return EndpointOptions{
		Getenv:  func(k string) string { return env[k] },
		HomeDir: func() (string, error) { return "/nonexistent-home", nil },
	}
}

func TestContextDirNameMatchesDockerCLI(t *testing.T) {
	// Verified against a real Docker Desktop install on 2026-11-27:
	// ~/.docker/contexts/meta/<this>/meta.json holds "desktop-linux".
	got := contextDirName("desktop-linux")
	want := "fe9c6bd7a66301f49ca9b6a70b217107cd1284598bfc254700c989b916da791e"
	if got != want {
		t.Fatalf("contextDirName = %s, want %s", got, want)
	}
}

func TestResolveEndpointPrecedence(t *testing.T) {
	const desktop = "unix:///Users/me/.docker/run/docker.sock"
	const colima = "unix:///Users/me/.colima/default/docker.sock"

	tests := []struct {
		name        string
		current     string
		host        string
		context     string
		env         map[string]string
		wantHost    string
		wantSource  string
		wantContext string
	}{
		{
			name:       "no config falls back to platform default",
			wantSource: "default",
		},
		{
			name:        "currentContext from config.json",
			current:     "desktop-linux",
			wantHost:    desktop,
			wantSource:  "context desktop-linux",
			wantContext: "desktop-linux",
		},
		{
			name:       "currentContext default means platform default",
			current:    "default",
			wantSource: "default",
		},
		{
			name:       "DOCKER_HOST beats currentContext",
			current:    "desktop-linux",
			env:        map[string]string{"DOCKER_HOST": "tcp://10.0.0.1:2375"},
			wantHost:   "tcp://10.0.0.1:2375",
			wantSource: "DOCKER_HOST",
		},
		{
			name:        "DOCKER_CONTEXT beats currentContext",
			current:     "desktop-linux",
			env:         map[string]string{"DOCKER_CONTEXT": "colima"},
			wantHost:    colima,
			wantSource:  "context colima",
			wantContext: "colima",
		},
		{
			name:       "DOCKER_HOST beats DOCKER_CONTEXT",
			env:        map[string]string{"DOCKER_HOST": "tcp://10.0.0.1:2375", "DOCKER_CONTEXT": "colima"},
			wantHost:   "tcp://10.0.0.1:2375",
			wantSource: "DOCKER_HOST",
		},
		{
			name:       "--host beats DOCKER_HOST and currentContext",
			current:    "desktop-linux",
			host:       "tcp://192.168.1.100:2375",
			env:        map[string]string{"DOCKER_HOST": "tcp://10.0.0.1:2375"},
			wantHost:   "tcp://192.168.1.100:2375",
			wantSource: "--host",
		},
		{
			name:        "--context beats DOCKER_HOST",
			context:     "colima",
			env:         map[string]string{"DOCKER_HOST": "tcp://10.0.0.1:2375"},
			wantHost:    colima,
			wantSource:  "context colima",
			wantContext: "colima",
		},
		{
			name:       "--context default still honors DOCKER_HOST",
			current:    "desktop-linux",
			context:    "default",
			env:        map[string]string{"DOCKER_HOST": "tcp://10.0.0.1:2375"},
			wantHost:   "tcp://10.0.0.1:2375",
			wantSource: "DOCKER_HOST",
		},
		{
			name:       "--context default without DOCKER_HOST is platform default",
			current:    "desktop-linux",
			context:    "default",
			wantSource: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeDockerConfig(t)
			f.addContext("desktop-linux", desktop, false)
			f.addContext("colima", colima, false)
			if tt.current != "" {
				f.setCurrent(tt.current)
			}
			o := f.opts(tt.env)
			o.Host = tt.host
			o.Context = tt.context

			ep, err := ResolveEndpoint(o)
			if err != nil {
				t.Fatalf("ResolveEndpoint: %v", err)
			}
			if ep.Host != tt.wantHost || ep.Source != tt.wantSource || ep.Context != tt.wantContext {
				t.Fatalf("got {Host:%q Source:%q Context:%q}, want {Host:%q Source:%q Context:%q}",
					ep.Host, ep.Source, ep.Context, tt.wantHost, tt.wantSource, tt.wantContext)
			}
		})
	}
}

func TestResolveEndpointErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(f *fakeDockerConfig)
		host    string
		context string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "--host and --context together",
			host:    "tcp://1.2.3.4:2375",
			context: "colima",
			wantErr: "cannot be used together",
		},
		{
			name:    "unknown --context",
			context: "nope",
			wantErr: `docker context "nope" (from --context) not found`,
		},
		{
			name:    "unknown DOCKER_CONTEXT",
			env:     map[string]string{"DOCKER_CONTEXT": "nope"},
			wantErr: `(from DOCKER_CONTEXT) not found`,
		},
		{
			name:    "currentContext points at a missing context",
			setup:   func(f *fakeDockerConfig) { f.setCurrent("gone") },
			wantErr: `docker context "gone" (from config.json currentContext) not found`,
		},
		{
			name:    "malformed config.json is an error, not a silent fallback",
			setup:   func(f *fakeDockerConfig) { f.write("config.json", "{not json") },
			wantErr: "parse ",
		},
		{
			name: "ssh endpoints are rejected with guidance",
			setup: func(f *fakeDockerConfig) {
				f.addContext("remote", "ssh://me@server", false)
				f.setCurrent("remote")
			},
			wantErr: "ssh endpoints are not supported",
		},
		{
			name: "context without a docker endpoint",
			setup: func(f *fakeDockerConfig) {
				f.write(filepath.Join("contexts", "meta", contextDirName("empty"), "meta.json"),
					`{"Name":"empty","Endpoints":{}}`)
				f.setCurrent("empty")
			},
			wantErr: "has no docker endpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeDockerConfig(t)
			if tt.setup != nil {
				tt.setup(f)
			}
			o := f.opts(tt.env)
			o.Host = tt.host
			o.Context = tt.context
			_, err := ResolveEndpoint(o)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestResolveEndpointUsesHomeDirWhenDockerConfigUnset(t *testing.T) {
	home := t.TempDir()
	f := &fakeDockerConfig{t: t, dir: filepath.Join(home, ".docker")}
	f.addContext("colima", "unix:///x/colima.sock", false)
	f.setCurrent("colima")

	ep, err := ResolveEndpoint(EndpointOptions{
		Getenv:  func(string) string { return "" },
		HomeDir: func() (string, error) { return home, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if ep.Host != "unix:///x/colima.sock" {
		t.Fatalf("Host = %q", ep.Host)
	}
}

func TestResolveEndpointTLS(t *testing.T) {
	f := newFakeDockerConfig(t)
	f.addContext("tls", "tcp://remote:2376", false)
	tlsDir := filepath.Join("contexts", "tls", contextDirName("tls"), "docker")
	f.write(filepath.Join(tlsDir, "ca.pem"), "ca")
	f.write(filepath.Join(tlsDir, "cert.pem"), "cert")
	f.write(filepath.Join(tlsDir, "key.pem"), "key")
	f.addContext("insecure", "tcp://remote:2376", true)
	f.addContext("plain", "tcp://remote:2375", false)

	o := f.opts(nil)
	o.Context = "tls"
	ep, err := ResolveEndpoint(o)
	if err != nil {
		t.Fatal(err)
	}
	if ep.TLS == nil || ep.TLS.CAFile == "" || ep.TLS.CertFile == "" || ep.TLS.KeyFile == "" || ep.TLS.SkipTLSVerify {
		t.Fatalf("TLS = %+v, want all files and verification on", ep.TLS)
	}

	o.Context = "insecure"
	ep, err = ResolveEndpoint(o)
	if err != nil {
		t.Fatal(err)
	}
	if ep.TLS == nil || !ep.TLS.SkipTLSVerify || ep.TLS.CAFile != "" {
		t.Fatalf("TLS = %+v, want skip-verify without files", ep.TLS)
	}

	o.Context = "plain"
	ep, err = ResolveEndpoint(o)
	if err != nil {
		t.Fatal(err)
	}
	if ep.TLS != nil {
		t.Fatalf("TLS = %+v, want nil for a plain context", ep.TLS)
	}
}

// A context endpoint must not be overridden by DOCKER_HOST in the SDK's
// FromEnv, otherwise --context would silently connect somewhere else.
func TestClientOptsContextIgnoresDockerHostEnv(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://10.9.9.9:2375")

	// tcp:// rather than unix:// so the test also runs on Windows, where the
	// SDK rejects unix sockets ("protocol not available").
	ep := Endpoint{Host: "tcp://127.0.0.1:23750", Source: "context x", Context: "x"}
	opts, err := clientOpts(ep)
	if err != nil {
		t.Fatal(err)
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	if got := cli.DaemonHost(); got != ep.Host {
		t.Fatalf("DaemonHost = %q, want context host %q", got, ep.Host)
	}

	// Non-context endpoints keep the existing FromEnv behavior.
	opts, err = clientOpts(Endpoint{Source: "DOCKER_HOST", Host: "tcp://10.9.9.9:2375"})
	if err != nil {
		t.Fatal(err)
	}
	cli2, err := client.NewClientWithOpts(opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer cli2.Close()
	if got := cli2.DaemonHost(); got != "tcp://10.9.9.9:2375" {
		t.Fatalf("DaemonHost = %q", got)
	}
}
