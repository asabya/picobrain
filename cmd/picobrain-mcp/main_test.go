package main

import (
	"errors"
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
