package picobrain

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestSplitPathSeq(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{",abc123,", 1},
		{",abc123,def456,", 2},
		{",abc,def,ghi,", 3},
		{",,", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := splitPathSeq(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitPathSeq(%q): got %d IDs, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestDepParserNotAvailable(t *testing.T) {
	brain := testBrain(t)

	// depParser should be nil in test brain (no spacy configured)
	if brain.depParser != nil {
		t.Error("expected depParser to be nil in test brain")
	}

	// ExtractTriples should return error
	_, err := brain.ExtractTriples(t.Context(), "test text")
	if err == nil {
		t.Error("expected error when depParser is nil")
	}
}

func TestResolveEntityEmpty(t *testing.T) {
	brain := testBrain(t)

	// resolveEntity with empty text should return empty
	result := brain.resolveEntity(t.Context(), "", 0.7)
	if result != "" {
		t.Errorf("expected empty result for empty entity, got %q", result)
	}
}

func TestFindSpacyServerUsesCacheDirOverride(t *testing.T) {
	overrideDir := t.TempDir()
	envDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(overrideDir, "server.py"), []byte("print('override')"), 0644); err != nil {
		t.Fatalf("write override server.py: %v", err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "server.py"), []byte("print('env')"), 0644); err != nil {
		t.Fatalf("write env server.py: %v", err)
	}

	oldEnv := os.Getenv("PICOBRAIN_SPACY_DIR")
	if err := os.Setenv("PICOBRAIN_SPACY_DIR", envDir); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PICOBRAIN_SPACY_DIR", oldEnv)
	})

	got, err := findSpacyServer(overrideDir)
	if err != nil {
		t.Fatalf("findSpacyServer: %v", err)
	}
	if got != overrideDir {
		t.Fatalf("expected override directory %q, got %q", overrideDir, got)
	}
}

func TestNewDepParserInstallsIntoCacheDirOverride(t *testing.T) {
	originalFinder := spacyServerFinder
	originalInstaller := spacyServerInstaller
	originalStarter := spacyServerStarter
	originalWaiter := spacyServerWaiter
	t.Cleanup(func() {
		spacyServerFinder = originalFinder
		spacyServerInstaller = originalInstaller
		spacyServerStarter = originalStarter
		spacyServerWaiter = originalWaiter
	})

	overrideDir := t.TempDir()
	installCalls := 0
	findCalls := 0
	spacyServerFinder = func(cacheDir string) (string, error) {
		findCalls++
		if findCalls == 1 {
			return "", errors.New("missing")
		}
		if cacheDir != overrideDir {
			t.Fatalf("expected override cache dir %q, got %q", overrideDir, cacheDir)
		}
		return overrideDir, nil
	}
	spacyServerInstaller = func(cacheDir string) error {
		installCalls++
		if cacheDir != overrideDir {
			t.Fatalf("expected install into override cache dir %q, got %q", overrideDir, cacheDir)
		}
		return nil
	}
	spacyServerStarter = func(serverDir string, port int) (*exec.Cmd, *bytes.Buffer, <-chan error, error) {
		waitCh := make(chan error, 1)
		return &exec.Cmd{}, &bytes.Buffer{}, waitCh, nil
	}
	spacyServerWaiter = func(baseURL string, httpClient *http.Client, waitCh <-chan error, startupLogs *bytes.Buffer, timeout time.Duration) error {
		return nil
	}

	parser, err := NewDepParser(overrideDir)
	if err != nil {
		t.Fatalf("NewDepParser: %v", err)
	}
	if parser == nil {
		t.Fatal("expected parser")
	}
	if installCalls != 1 {
		t.Fatalf("expected one install attempt, got %d", installCalls)
	}
}

func TestNewDepParserHealthcheckFailureStopsProcess(t *testing.T) {
	originalFinder := spacyServerFinder
	originalInstaller := spacyServerInstaller
	originalStarter := spacyServerStarter
	originalWaiter := spacyServerWaiter
	originalStopper := spacyProcessStopper
	t.Cleanup(func() {
		spacyServerFinder = originalFinder
		spacyServerInstaller = originalInstaller
		spacyServerStarter = originalStarter
		spacyServerWaiter = originalWaiter
		spacyProcessStopper = originalStopper
	})

	spacyServerFinder = func(cacheDir string) (string, error) {
		return t.TempDir(), nil
	}
	spacyServerInstaller = func(cacheDir string) error {
		return nil
	}
	cmd := &exec.Cmd{}
	waitCh := make(chan error, 1)
	spacyServerStarter = func(serverDir string, port int) (*exec.Cmd, *bytes.Buffer, <-chan error, error) {
		return cmd, &bytes.Buffer{}, waitCh, nil
	}
	spacyServerWaiter = func(baseURL string, httpClient *http.Client, waitCh <-chan error, startupLogs *bytes.Buffer, timeout time.Duration) error {
		return errors.New("timed out")
	}

	stopCalls := 0
	spacyProcessStopper = func(gotCmd *exec.Cmd, gotWaitCh <-chan error) error {
		stopCalls++
		if gotCmd != cmd {
			t.Fatalf("expected stopper to receive startup command")
		}
		return nil
	}

	parser, err := NewDepParser("")
	if err == nil {
		if parser != nil {
			parser.Close()
		}
		t.Fatal("expected health-check failure")
	}
	if stopCalls != 1 {
		t.Fatalf("expected one cleanup stop call, got %d", stopCalls)
	}
	if !errors.Is(err, errSpacyStartupHealthcheck) {
		t.Fatalf("expected healthcheck sentinel, got %v", err)
	}
}
