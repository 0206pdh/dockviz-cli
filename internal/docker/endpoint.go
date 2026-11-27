// endpoint.go resolves which Docker daemon to talk to, following the same
// precedence as the docker CLI so that Docker Desktop, Colima, OrbStack and
// remote contexts work without extra flags or environment variables.
package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// defaultContextName is the docker CLI's built-in context. It has no metadata
// on disk and means "use DOCKER_HOST or the platform default socket".
const defaultContextName = "default"

// EndpointOptions are the inputs to ResolveEndpoint. Getenv and HomeDir are
// injectable so tests never read the real environment or ~/.docker.
type EndpointOptions struct {
	Host    string // --host flag
	Context string // --context flag
	Getenv  func(string) string
	HomeDir func() (string, error)
}

// Endpoint is the resolved daemon address and where it came from.
type Endpoint struct {
	// Host is the daemon URL. Empty means "let the SDK use DOCKER_HOST or the
	// platform default" (client.FromEnv).
	Host string
	// Source describes which setting won, for error messages and display,
	// e.g. "--host", "DOCKER_HOST", "context desktop-linux", "default".
	Source string
	// Context is the context name when the endpoint came from a context.
	Context string
	// TLS is set when a context carries TLS material or skips verification.
	TLS *EndpointTLS
}

// EndpointTLS holds the TLS files stored for a context.
type EndpointTLS struct {
	CAFile        string
	CertFile      string
	KeyFile       string
	SkipTLSVerify bool
}

// FromContext reports whether the endpoint was read from a non-default
// context. Such endpoints must not pick up DOCKER_HOST/DOCKER_CERT_PATH from
// the environment, matching the docker CLI.
func (e Endpoint) FromContext() bool { return e.Context != "" }

// ResolveEndpoint picks the daemon endpoint with the docker CLI's precedence:
//
//  1. --context flag
//  2. --host flag
//  3. DOCKER_HOST
//  4. DOCKER_CONTEXT
//  5. currentContext in $DOCKER_CONFIG/config.json (default ~/.docker)
//  6. the platform default socket
//
// --host and --context together are rejected, as the docker CLI does.
func ResolveEndpoint(opts EndpointOptions) (Endpoint, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	homeDir := opts.HomeDir
	if homeDir == nil {
		homeDir = os.UserHomeDir
	}

	if opts.Host != "" && opts.Context != "" {
		return Endpoint{}, errors.New("--host and --context cannot be used together")
	}

	if opts.Context != "" {
		return endpointForContext(opts.Context, "--context", getenv, homeDir)
	}
	if opts.Host != "" {
		return Endpoint{Host: opts.Host, Source: "--host"}, nil
	}
	if h := getenv("DOCKER_HOST"); h != "" {
		return Endpoint{Host: h, Source: "DOCKER_HOST"}, nil
	}
	if name := getenv("DOCKER_CONTEXT"); name != "" {
		return endpointForContext(name, "DOCKER_CONTEXT", getenv, homeDir)
	}

	configDir, err := dockerConfigDir(getenv, homeDir)
	if err != nil {
		// No home directory: nothing to read, fall back like the CLI would.
		return Endpoint{Source: "default"}, nil
	}
	name, err := currentContext(configDir)
	if err != nil {
		return Endpoint{}, err
	}
	if name == "" || name == defaultContextName {
		return Endpoint{Source: "default"}, nil
	}
	return loadContext(configDir, name, "config.json currentContext")
}

func endpointForContext(name, via string, getenv func(string) string, homeDir func() (string, error)) (Endpoint, error) {
	if name == defaultContextName {
		// The built-in default context still honors DOCKER_HOST.
		if h := getenv("DOCKER_HOST"); h != "" {
			return Endpoint{Host: h, Source: "DOCKER_HOST"}, nil
		}
		return Endpoint{Source: "default"}, nil
	}
	configDir, err := dockerConfigDir(getenv, homeDir)
	if err != nil {
		return Endpoint{}, fmt.Errorf("cannot locate docker config directory for context %q: %w", name, err)
	}
	return loadContext(configDir, name, via)
}

func dockerConfigDir(getenv func(string) string, homeDir func() (string, error)) (string, error) {
	if dir := getenv("DOCKER_CONFIG"); dir != "" {
		return dir, nil
	}
	home, err := homeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("home directory unavailable: %w", err)
	}
	return filepath.Join(home, ".docker"), nil
}

// currentContext returns config.json's currentContext, or "" when the file
// does not exist. A malformed file is an error rather than a silent fallback,
// because silently connecting to a different daemon is worse than failing.
func currentContext(configDir string) (string, error) {
	path := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	var cfg struct {
		CurrentContext string `json:"currentContext"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg.CurrentContext, nil
}

// contextDirName is the on-disk directory name the docker CLI uses for a
// context: the hex SHA-256 of its name.
func contextDirName(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])
}

type contextMeta struct {
	Name      string `json:"Name"`
	Endpoints map[string]struct {
		Host          string `json:"Host"`
		SkipTLSVerify bool   `json:"SkipTLSVerify"`
	} `json:"Endpoints"`
}

func loadContext(configDir, name, via string) (Endpoint, error) {
	id := contextDirName(name)
	metaPath := filepath.Join(configDir, "contexts", "meta", id, "meta.json")
	data, err := os.ReadFile(metaPath)
	if errors.Is(err, os.ErrNotExist) {
		return Endpoint{}, fmt.Errorf("docker context %q (from %s) not found; run `docker context ls`", name, via)
	}
	if err != nil {
		return Endpoint{}, fmt.Errorf("read docker context %q: %w", name, err)
	}
	var meta contextMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Endpoint{}, fmt.Errorf("parse docker context %q (%s): %w", name, metaPath, err)
	}
	ep, ok := meta.Endpoints["docker"]
	if !ok || ep.Host == "" {
		return Endpoint{}, fmt.Errorf("docker context %q has no docker endpoint", name)
	}
	if strings.HasPrefix(strings.ToLower(ep.Host), "ssh://") {
		return Endpoint{}, fmt.Errorf("docker context %q uses %s; ssh endpoints are not supported yet — use --host with a unix:// or tcp:// address", name, ep.Host)
	}

	out := Endpoint{
		Host:    ep.Host,
		Source:  "context " + name,
		Context: name,
	}

	tlsDir := filepath.Join(configDir, "contexts", "tls", id, "docker")
	t := EndpointTLS{SkipTLSVerify: ep.SkipTLSVerify}
	found := false
	for file, dst := range map[string]*string{"ca.pem": &t.CAFile, "cert.pem": &t.CertFile, "key.pem": &t.KeyFile} {
		p := filepath.Join(tlsDir, file)
		if _, err := os.Stat(p); err == nil {
			*dst = p
			found = true
		}
	}
	if found || t.SkipTLSVerify {
		out.TLS = &t
	}
	return out, nil
}
