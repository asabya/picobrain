package picobrain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// ExpectedEmbeddingDim is the expected dimensionality of embedding vectors.
	ExpectedEmbeddingDim = 768
)

var errModelNotFound = errors.New("model not found")

var downloadModelFile = downloadModel

// Embedder is the interface for text embedding providers.
// All implementations must return exactly 768-dimensional vectors.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Close() error
}

// OllamaEmbedder provides embeddings via a remote Ollama server.
// Deprecated: retained for backward compatibility; prefer LocalEmbedder.
type OllamaEmbedder struct {
	url   string
	model string
}

// NewOllamaEmbedder creates a new OllamaEmbedder.
func NewOllamaEmbedder(ollamaURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{url: ollamaURL, model: model}
}

// Embed generates an embedding for the given text via Ollama.
func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody, err := json.Marshal(map[string]string{
		"model":  e.model,
		"prompt": text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.url+"/api/embeddings", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama returned empty embedding")
	}

	return result.Embedding, nil
}

// Close is a no-op for OllamaEmbedder.
func (e *OllamaEmbedder) Close() error {
	return nil
}

const (
	localEmbedStartupTimeout = 30 * time.Second
	localEmbedRequestTimeout = 60 * time.Second
)

// LocalEmbedder provides embeddings using a local GGUF model via llama-server.
type LocalEmbedder struct {
	baseURL    string
	httpClient *http.Client
	cmd        *exec.Cmd
	waitCh     <-chan error
}

// NewLocalEmbedder creates a new LocalEmbedder that serves a local GGUF model through llama-server.
// It will auto-download the model from HuggingFace if not cached and autoDownload is true.
func NewLocalEmbedder(modelName, cacheDir string, autoDownload bool) (*LocalEmbedder, error) {
	modelPath, err := ensureModel(modelName, cacheDir, autoDownload)
	if err != nil {
		return nil, err
	}

	serverPath, err := findLlamaServerBinary()
	if err != nil {
		return nil, err
	}

	port, err := reservePort()
	if err != nil {
		return nil, fmt.Errorf("reserve llama-server port: %w", err)
	}

	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	cmd, startupLogs, waitCh, err := startLlamaServer(serverPath, modelPath, port)
	if err != nil {
		return nil, fmt.Errorf("start llama-server: %w", err)
	}

	startupHTTPClient := &http.Client{Timeout: time.Second}
	if err := waitForServer(baseURL, startupHTTPClient, waitCh, startupLogs, localEmbedStartupTimeout); err != nil {
		_ = stopProcess(cmd, waitCh)
		return nil, err
	}

	return &LocalEmbedder{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: localEmbedRequestTimeout,
		},
		cmd:    cmd,
		waitCh: waitCh,
	}, nil
}

func ensureModel(modelName, cacheDir string, autoDownload bool) (string, error) {
	modelPath, err := resolveModelPath(modelName, cacheDir)
	if err == nil {
		return modelPath, nil
	}
	if !errors.Is(err, errModelNotFound) {
		return "", fmt.Errorf("resolve model path: %w", err)
	}
	if !autoDownload {
		return "", fmt.Errorf("model %q is not cached at %s: %w", modelName, modelPath, err)
	}

	repo, filename, err := modelToHuggingFace(modelName)
	if err != nil {
		return "", fmt.Errorf("resolve download source for model %q: %w", modelName, err)
	}
	if err := downloadModelFile(repo, filename, modelPath); err != nil {
		return "", fmt.Errorf("download model %q to %s: %w", modelName, modelPath, err)
	}

	return modelPath, nil
}

// Embed generates an embedding for the given text using the local model.
func (e *LocalEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody, err := json.Marshal(map[string]string{
		"content": text,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embedding", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("local embedding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("llama-server returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}

	if len(result.Embedding) != ExpectedEmbeddingDim {
		return nil, fmt.Errorf("expected %d dimensions, got %d", ExpectedEmbeddingDim, len(result.Embedding))
	}

	return result.Embedding, nil
}

// Close stops the underlying llama-server process.
func (e *LocalEmbedder) Close() error {
	if e.cmd == nil || e.cmd.Process == nil {
		return nil
	}
	return stopProcess(e.cmd, e.waitCh)
}

func findLlamaServerBinary() (string, error) {
	if bin := os.Getenv("PICOBRAIN_LLAMA_SERVER_BIN"); bin != "" {
		return bin, nil
	}

	bin, err := exec.LookPath("llama-server")
	if err != nil {
		return "", fmt.Errorf("find llama-server binary: %w", err)
	}
	return bin, nil
}

func reservePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address %T", ln.Addr())
	}
	return addr.Port, nil
}

func startLlamaServer(serverPath, modelPath string, port int) (*exec.Cmd, *bytes.Buffer, <-chan error, error) {
	args := []string{
		"-m", modelPath,
		"--embedding",
		"--pooling", "cls",
		"--port", strconv.Itoa(port),
		"--host", "127.0.0.1",
	}

	cmd := exec.Command(serverPath, args...)
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

func waitForServer(baseURL string, httpClient *http.Client, waitCh <-chan error, startupLogs *bytes.Buffer, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastStatus string

	for time.Now().Before(deadline) {
		select {
		case err := <-waitCh:
			return fmt.Errorf("llama-server exited during startup: %s: %s", formatWaitError(err), strings.TrimSpace(startupLogs.String()))
		default:
		}

		req, err := http.NewRequest(http.MethodGet, baseURL+"/health", nil)
		if err != nil {
			return fmt.Errorf("create llama-server health request: %w", err)
		}
		resp, err := httpClient.Do(req)
		if err == nil {
			resp.Body.Close()
			lastStatus = resp.Status
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		} else {
			lastStatus = err.Error()
		}

		time.Sleep(250 * time.Millisecond)
	}

	if lastStatus != "" {
		return fmt.Errorf("timed out waiting for llama-server readiness (%s): %s", lastStatus, strings.TrimSpace(startupLogs.String()))
	}
	return fmt.Errorf("timed out waiting for llama-server readiness: %s", strings.TrimSpace(startupLogs.String()))
}

func stopProcess(cmd *exec.Cmd, waitCh <-chan error) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}

	select {
	case err := <-waitCh:
		if err != nil && !isExpectedProcessExit(err) {
			return err
		}
		return nil
	case <-time.After(5 * time.Second):
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		err := <-waitCh
		if err != nil && !isExpectedProcessExit(err) {
			return err
		}
		return nil
	}
}

func formatWaitError(err error) string {
	if err == nil {
		return "process exited cleanly"
	}
	return err.Error()
}

func isExpectedProcessExit(err error) bool {
	if err == nil {
		return true
	}

	return strings.Contains(err.Error(), "signal: terminated") ||
		strings.Contains(err.Error(), "killed")
}

// resolveModelPath returns the local file path for a model.
func resolveModelPath(modelName, cacheDir string) (string, error) {
	_, filename, err := modelToHuggingFace(modelName)
	if err != nil {
		return "", err
	}
	path := filepath.Join(cacheDir, filename)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return path, errModelNotFound
		}
		return path, fmt.Errorf("stat model: %w", err)
	}
	return path, nil
}

// modelToHuggingFace returns the HuggingFace repo and filename for a model name.
func modelToHuggingFace(modelName string) (repo, filename string, err error) {
	switch modelName {
	case "nomic-embed-text", "nomic-embed-text-v1.5":
		repo = "nomic-ai/nomic-embed-text-v1.5-GGUF"
		filename = "nomic-embed-text-v1.5.Q8_0.gguf"
	default:
		return "", "", fmt.Errorf("unsupported model: %s", modelName)
	}
	return repo, filename, nil
}

// downloadModel downloads a file from HuggingFace to destPath.
func downloadModel(repo, filename, destPath string) error {
	url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repo, filename)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Ensure cache directory exists
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	// Download to temp file then rename
	tmp, err := os.CreateTemp(dir, ".download-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	_, err = io.Copy(tmp, resp.Body)
	if closeErr := tmp.Close(); closeErr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("write: %w", closeErr)
	}
	if err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("write: %w", err)
	}

	// Ensure directory exists for rename (some filesystems need this)
	if err := os.MkdirAll(dir, 0755); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("create cache dir: %w", err)
	}

	if err := os.Rename(tmpName, destPath); err != nil {
		// Fallback: copy if rename fails (cross-device)
		if strings.Contains(err.Error(), "invalid argument") || strings.Contains(err.Error(), "cross-device") {
			data, rErr := os.ReadFile(tmpName)
			if rErr != nil {
				os.Remove(tmpName)
				return fmt.Errorf("read temp: %w", rErr)
			}
			if wErr := os.WriteFile(destPath, data, 0644); wErr != nil {
				os.Remove(tmpName)
				return fmt.Errorf("write dest: %w", wErr)
			}
			os.Remove(tmpName)
			return nil
		}
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}
