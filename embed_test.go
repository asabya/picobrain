package picobrain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
