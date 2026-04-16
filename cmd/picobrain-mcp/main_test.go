package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asabya/picobrain"
)

func TestRunServerEFailsFastWhenBrainInitFails(t *testing.T) {
	originalNewBrain := newBrain
	newBrain = func(cfg picobrain.Config) (*picobrain.Brain, error) {
		return nil, errors.New("spacy boot failed")
	}
	t.Cleanup(func() {
		newBrain = originalNewBrain
	})

	err := runServerE([]string{"--db", filepath.Join(t.TempDir(), "brain.db"), "--port", "0"})
	if err == nil {
		t.Fatal("expected runServerE to fail")
	}
	if !strings.Contains(err.Error(), "initialize brain") {
		t.Fatalf("expected initialize brain error, got %v", err)
	}
}

func TestExportCommandFailsFastWhenBrainInitFails(t *testing.T) {
	originalNewBrain := newBrain
	newBrain = func(cfg picobrain.Config) (*picobrain.Brain, error) {
		return nil, errors.New("spacy boot failed")
	}
	t.Cleanup(func() {
		newBrain = originalNewBrain
	})

	cmd := newExportCommand()
	if err := cmd.Parse([]string{"--db", filepath.Join(t.TempDir(), "brain.db")}); err != nil {
		t.Fatalf("parse: %v", err)
	}

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected export command to fail")
	}
	if !strings.Contains(err.Error(), "initialize brain") {
		t.Fatalf("expected initialize brain error, got %v", err)
	}
}

func TestImportCommandFailsFastWhenBrainInitFails(t *testing.T) {
	originalNewBrain := newBrain
	newBrain = func(cfg picobrain.Config) (*picobrain.Brain, error) {
		return nil, errors.New("spacy boot failed")
	}
	t.Cleanup(func() {
		newBrain = originalNewBrain
	})

	inputFile := filepath.Join(t.TempDir(), "import.jsonl")
	if err := os.WriteFile(inputFile, []byte("{\"content\":\"hello\"}\n"), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	cmd := newImportCommand()
	if err := cmd.Parse([]string{"--input", inputFile, "--db", filepath.Join(t.TempDir(), "brain.db")}); err != nil {
		t.Fatalf("parse: %v", err)
	}

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected import command to fail")
	}
	if !strings.Contains(err.Error(), "initialize brain") {
		t.Fatalf("expected initialize brain error, got %v", err)
	}
}
