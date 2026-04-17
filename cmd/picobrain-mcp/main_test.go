package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asabya/picobrain"
	"github.com/mark3labs/mcp-go/server"
)

func withNewBrainStub(t *testing.T, stub func(picobrain.Config) (*picobrain.Brain, error)) {
	t.Helper()
	prev := newBrain
	newBrain = stub
	t.Cleanup(func() {
		newBrain = prev
	})
}

func TestRunServerEFailFastWhenBrainInitFails(t *testing.T) {
	sentinel := errors.New("spacy startup failed")
	withNewBrainStub(t, func(cfg picobrain.Config) (*picobrain.Brain, error) {
		return nil, sentinel
	})

	startCalled := false
	prevStart := startHTTPServer
	startHTTPServer = func(httpServer *server.StreamableHTTPServer, addr string) error {
		startCalled = true
		return nil
	}
	t.Cleanup(func() {
		startHTTPServer = prevStart
	})

	err := runServerE([]string{"-db", filepath.Join(t.TempDir(), "brain.db")})
	if err == nil {
		t.Fatal("expected runServerE to return an error")
	}
	if !strings.Contains(err.Error(), "failed to initialize brain") {
		t.Fatalf("expected brain init error, got %v", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
	if startCalled {
		t.Fatal("expected server startup to be skipped when brain init fails")
	}
}

func TestExportCommandFailFastWhenBrainInitFails(t *testing.T) {
	sentinel := errors.New("spacy startup failed")
	withNewBrainStub(t, func(cfg picobrain.Config) (*picobrain.Brain, error) {
		return nil, sentinel
	})

	cmd := newExportCommand()
	if err := cmd.Parse([]string{"-db", filepath.Join(t.TempDir(), "brain.db")}); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected export command to fail")
	}
	if !strings.Contains(err.Error(), "initialize brain") {
		t.Fatalf("expected initialize brain error, got %v", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
}

func TestImportCommandFailFastWhenBrainInitFails(t *testing.T) {
	sentinel := errors.New("spacy startup failed")
	withNewBrainStub(t, func(cfg picobrain.Config) (*picobrain.Brain, error) {
		return nil, sentinel
	})

	inputPath := filepath.Join(t.TempDir(), "input.jsonl")
	if err := os.WriteFile(inputPath, []byte("{\"content\":\"hello\"}\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := newImportCommand()
	if err := cmd.Parse([]string{"-input", inputPath, "-db", filepath.Join(t.TempDir(), "brain.db")}); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected import command to fail")
	}
	if !strings.Contains(err.Error(), "initialize brain") {
		t.Fatalf("expected initialize brain error, got %v", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
}

func TestRunServerEFailsFastOnMalformedAuth(t *testing.T) {
	brainCalled := false
	withNewBrainStub(t, func(cfg picobrain.Config) (*picobrain.Brain, error) {
		brainCalled = true
		return nil, errors.New("brain init should not be reached")
	})

	startCalled := false
	prevStart := startHTTPServer
	startHTTPServer = func(httpServer *server.StreamableHTTPServer, addr string) error {
		startCalled = true
		return nil
	}
	t.Cleanup(func() {
		startHTTPServer = prevStart
	})

	prevAuthStart := startAuthHTTPServer
	startAuthHTTPServer = func(httpServer *http.Server, addr string) error {
		startCalled = true
		return nil
	}
	t.Cleanup(func() {
		startAuthHTTPServer = prevAuthStart
	})

	err := runServerE([]string{"-auth", "noseparator"})
	if err == nil {
		t.Fatal("expected malformed auth to fail startup")
	}
	if !strings.Contains(err.Error(), "--auth=username:password") {
		t.Fatalf("expected auth hint, got %v", err)
	}
	if brainCalled {
		t.Fatal("expected malformed auth to fail before brain initialization")
	}
	if startCalled {
		t.Fatal("expected malformed auth to fail before server start")
	}
}

func TestParseAuthFlagAcceptsUsernameColonPassword(t *testing.T) {
	creds, err := parseAuthFlag("alice:secret")
	if err != nil {
		t.Fatalf("parseAuthFlag: %v", err)
	}
	if creds.username != "alice" || creds.password != "secret" {
		t.Fatalf("unexpected creds: %#v", creds)
	}
}

func TestParseAuthFlagSplitsOnFirstColon(t *testing.T) {
	creds, err := parseAuthFlag("alice:sec:ret")
	if err != nil {
		t.Fatalf("parseAuthFlag: %v", err)
	}
	if creds.username != "alice" || creds.password != "sec:ret" {
		t.Fatalf("unexpected creds: %#v", creds)
	}
}

func TestParseAuthFlagRejectsMalformedValue(t *testing.T) {
	for _, input := range []string{"", "noseparator", ":secret", "alice:"} {
		if _, err := parseAuthFlag(input); err == nil {
			t.Fatalf("expected parseAuthFlag to reject %q", input)
		}
	}
}

func TestBasicAuthWrapperProtectsMCPMethods(t *testing.T) {
	nextCalled := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	})

	h := wrapBasicAuth(next, basicAuthCredentials{username: "alice", password: "secret"})

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		t.Run(method+"_missing_credentials", func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/mcp", nil)
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got == "" {
				t.Fatal("expected WWW-Authenticate challenge header")
			}
		})

		t.Run(method+"_wrong_credentials", func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/mcp", nil)
			req.SetBasicAuth("alice", "wrong")
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
		})

		t.Run(method+"_valid_credentials", func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/mcp", nil)
			req.SetBasicAuth("alice", "secret")
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
		})
	}

	if nextCalled == 0 {
		t.Fatal("expected wrapped handler to be reachable with valid credentials")
	}
}
