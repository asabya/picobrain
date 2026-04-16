package picobrain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	depParserStartupTimeout = 60 * time.Second
	depParserRequestTimeout = 30 * time.Second
)

// Triple represents an extracted (head, relation, tail) from dependency parsing.
type Triple struct {
	Head     string `json:"head"`
	Relation string `json:"relation"`
	Tail     string `json:"tail"`
}

// DepParser runs dependency parsing via a local SpaCy sidecar server.
// It follows the same pattern as LocalEmbedder: start subprocess, health check, HTTP calls.
type DepParser struct {
	baseURL    string
	httpClient *http.Client
	cmd        *exec.Cmd
	waitCh     <-chan error
}

// NewDepParser starts the SpaCy sidecar server and returns a connected client.
// If the server is not installed, it returns an error (unless autoInstall is true).
func NewDepParser(cacheDir string, autoInstall bool) (*DepParser, error) {
	serverDir, err := findSpacyServer()
	if err != nil {
		if !autoInstall {
			return nil, fmt.Errorf("spacy server not found: %w (set auto_install_spacy=true to auto-install)", err)
		}
		if err := installSpacyServer(cacheDir); err != nil {
			return nil, fmt.Errorf("install spacy server: %w", err)
		}
		serverDir, err = findSpacyServer()
		if err != nil {
			return nil, err
		}
	}

	port, err := reservePort()
	if err != nil {
		return nil, fmt.Errorf("reserve spacy server port: %w", err)
	}

	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	cmd, startupLogs, waitCh, err := startSpacyServer(serverDir, port)
	if err != nil {
		return nil, fmt.Errorf("start spacy server: %w", err)
	}

	startupHTTPClient := &http.Client{Timeout: time.Second}
	if err := waitForServer(baseURL, startupHTTPClient, waitCh, startupLogs, depParserStartupTimeout); err != nil {
		_ = stopProcess(cmd, waitCh)
		return nil, err
	}

	return &DepParser{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: depParserRequestTimeout,
		},
		cmd:    cmd,
		waitCh: waitCh,
	}, nil
}

// Parse extracts (head, relation, tail) triples from text via the SpaCy server.
func (d *DepParser) Parse(ctx context.Context, text string) ([]Triple, error) {
	reqBody, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, fmt.Errorf("marshal parse request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/parse", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create parse request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spacy parse request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("spacy server returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Triples []Triple `json:"triples"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode parse response: %w", err)
	}

	return result.Triples, nil
}

// Close stops the SpaCy server process.
func (d *DepParser) Close() error {
	if d.cmd == nil || d.cmd.Process == nil {
		return nil
	}
	return stopProcess(d.cmd, d.waitCh)
}

// findSpacyServer looks for the SpaCy server installation.
// It checks: PICOBRAIN_SPACY_DIR env var, then ~/.picobrain/spacy/server.py.
func findSpacyServer() (string, error) {
	if dir := os.Getenv("PICOBRAIN_SPACY_DIR"); dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "server.py")); err == nil {
			return dir, nil
		}
	}

	home, _ := os.UserHomeDir()
	defaultDir := filepath.Join(home, ".picobrain", "spacy")
	if _, err := os.Stat(filepath.Join(defaultDir, "server.py")); err == nil {
		return defaultDir, nil
	}

	return "", errors.New("spacy server not found in PICOBRAIN_SPACY_DIR or ~/.picobrain/spacy")
}

// installSpacyServer runs the install script to set up the SpaCy virtualenv.
func installSpacyServer(cacheDir string) error {
	if cacheDir == "" {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".picobrain", "spacy")
	}

	// Find install.sh in the bundled spacy_server directory
	execDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Try multiple locations for install.sh
	installPaths := []string{
		filepath.Join(execDir, "spacy_server", "install.sh"),
		filepath.Join(execDir, "..", "spacy_server", "install.sh"),
	}

	// Also check relative to the executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		installPaths = append(installPaths,
			filepath.Join(exeDir, "spacy_server", "install.sh"),
			filepath.Join(exeDir, "..", "spacy_server", "install.sh"),
		)
	}

	var installScript string
	for _, p := range installPaths {
		if _, err := os.Stat(p); err == nil {
			installScript = p
			break
		}
	}

	if installScript == "" {
		return fmt.Errorf("install.sh not found; please install SpaCy manually: see spacy_server/README.md")
	}

	cmd := exec.Command("bash", installScript)
	cmd.Env = append(os.Environ(), "PICOBRAIN_SPACY_DIR="+cacheDir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run install.sh: %w", err)
	}
	return nil
}

// startSpacyServer starts the SpaCy FastAPI server as a subprocess.
func startSpacyServer(serverDir string, port int) (*exec.Cmd, *bytes.Buffer, <-chan error, error) {
	venvPython := filepath.Join(serverDir, "venv", "bin", "python")
	if _, err := os.Stat(venvPython); err != nil {
		return nil, nil, nil, fmt.Errorf("venv python not found at %s: %w", venvPython, err)
	}

	args := []string{
		"-m", "uvicorn",
		"server:app",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--log-level", "warning",
	}

	cmd := exec.Command(venvPython, args...)
	cmd.Dir = serverDir
	var startupLogs bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stderr, &startupLogs)
	cmd.Stderr = io.MultiWriter(os.Stderr, &startupLogs)

	if err := cmd.Start(); err != nil {
		return nil, nil, nil, err
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	return cmd, &startupLogs, waitCh, nil
}