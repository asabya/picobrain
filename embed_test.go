package picobrain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOllamaEmbedder_Embed(t *testing.T) {
	want := []float32{0.1, 0.2, 0.3, 0.4, 0.5}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("expected /api/embeddings, got %s", r.URL.Path)
		}

		var req struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.Model != "nomic-embed-text" {
			t.Errorf("expected model nomic-embed-text, got %s", req.Model)
		}
		if req.Prompt != "hello world" {
			t.Errorf("expected prompt 'hello world', got %s", req.Prompt)
		}

		json.NewEncoder(w).Encode(map[string]any{
			"embedding": want,
		})
	}))
	defer server.Close()

	embedder := NewOllamaEmbedder(server.URL, "nomic-embed-text")
	got, err := embedder.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d dimensions, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dimension %d: expected %f, got %f", i, want[i], got[i])
		}
	}
}

func TestOllamaEmbedder_EmbedConnectionError(t *testing.T) {
	embedder := NewOllamaEmbedder("http://localhost:1", "nomic-embed-text")
	_, err := embedder.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestEnsureModelUsesCachedFile(t *testing.T) {
	cacheDir := t.TempDir()
	modelPath := filepath.Join(cacheDir, "nomic-embed-text-v1.5.Q8_0.gguf")
	if err := os.WriteFile(modelPath, []byte("cached"), 0o644); err != nil {
		t.Fatalf("write cached model: %v", err)
	}

	got, err := ensureModel("nomic-embed-text", cacheDir, false)
	if err != nil {
		t.Fatalf("ensureModel: %v", err)
	}
	if got != modelPath {
		t.Fatalf("expected %s, got %s", modelPath, got)
	}
}

func TestEnsureModelMissingWithoutAutoDownload(t *testing.T) {
	cacheDir := t.TempDir()

	_, err := ensureModel("nomic-embed-text", cacheDir, false)
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "is not cached") {
		t.Fatalf("expected missing cache error, got %v", err)
	}
}

func TestEnsureModelDownloadsWhenMissing(t *testing.T) {
	cacheDir := t.TempDir()

	prev := downloadModelFile
	downloadModelFile = func(repo, filename, destPath string) error {
		if repo != "nomic-ai/nomic-embed-text-v1.5-GGUF" {
			t.Fatalf("unexpected repo %q", repo)
		}
		if filename != "nomic-embed-text-v1.5.Q8_0.gguf" {
			t.Fatalf("unexpected filename %q", filename)
		}
		return os.WriteFile(destPath, []byte("downloaded"), 0o644)
	}
	t.Cleanup(func() {
		downloadModelFile = prev
	})

	got, err := ensureModel("nomic-embed-text-v1.5", cacheDir, true)
	if err != nil {
		t.Fatalf("ensureModel: %v", err)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("expected downloaded model at %s: %v", got, err)
	}
}

func TestEnsureModelDownloadFailure(t *testing.T) {
	cacheDir := t.TempDir()

	prev := downloadModelFile
	downloadModelFile = func(repo, filename, destPath string) error {
		return errors.New("network down")
	}
	t.Cleanup(func() {
		downloadModelFile = prev
	})

	_, err := ensureModel("nomic-embed-text", cacheDir, true)
	if err == nil {
		t.Fatal("expected download error")
	}
	if !strings.Contains(err.Error(), "download model") {
		t.Fatalf("expected download context, got %v", err)
	}
}

func TestEnsureModelUnsupportedModel(t *testing.T) {
	cacheDir := t.TempDir()

	_, err := ensureModel("unknown-model", cacheDir, true)
	if err == nil {
		t.Fatal("expected unsupported model error")
	}
	if !strings.Contains(err.Error(), "unsupported model") {
		t.Fatalf("expected unsupported model error, got %v", err)
	}
}

func TestLocalEmbedderEmbed(t *testing.T) {
	want := make([]float32, ExpectedEmbeddingDim)
	want[0] = 1.0
	want[ExpectedEmbeddingDim-1] = 0.5

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embedding" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var req struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Content != "hello world" {
			t.Fatalf("unexpected content %q", req.Content)
		}

		if err := json.NewEncoder(w).Encode(map[string]any{"embedding": want}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	embedder := &LocalEmbedder{
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: time.Second,
		},
	}

	got, err := embedder.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d dimensions, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dimension %d: expected %f, got %f", i, want[i], got[i])
		}
	}
}

func TestWaitForServerWaitsForHealthOK(t *testing.T) {
	var healthChecks int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		healthChecks++
		if healthChecks < 3 {
			http.Error(w, "loading", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	waitCh := make(chan error, 1)
	start := time.Now()
	err := waitForServer(server.URL, &http.Client{Timeout: 200 * time.Millisecond}, waitCh, &bytes.Buffer{}, 2*time.Second)
	if err != nil {
		t.Fatalf("waitForServer: %v", err)
	}
	if healthChecks < 3 {
		t.Fatalf("expected multiple health checks, got %d", healthChecks)
	}
	if time.Since(start) < 400*time.Millisecond {
		t.Fatalf("waitForServer returned before health became ready")
	}
}

func TestWaitForServerReturnsPromptlyOnProcessExit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "loading", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	waitCh := make(chan error, 1)
	waitCh <- errors.New("exit status 1")

	start := time.Now()
	err := waitForServer(server.URL, &http.Client{Timeout: 200 * time.Millisecond}, waitCh, bytes.NewBufferString("boom"), 5*time.Second)
	if err == nil {
		t.Fatal("expected startup failure")
	}
	if !strings.Contains(err.Error(), "exited during startup") {
		t.Fatalf("expected startup exit error, got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected startup logs in error, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("waitForServer did not return promptly after process exit")
	}
}

func TestWaitForServerTimeoutIncludesLastStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "loading", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	waitCh := make(chan error)

	err := waitForServer(server.URL, &http.Client{Timeout: 200 * time.Millisecond}, waitCh, bytes.NewBufferString("still loading"), 750*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "503 Service Unavailable") {
		t.Fatalf("expected last status in timeout error, got %v", err)
	}
	if !strings.Contains(err.Error(), "still loading") {
		t.Fatalf("expected startup logs in timeout error, got %v", err)
	}
}
