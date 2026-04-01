# Unified Release Pipeline Design

## Goal

Merge goreleaser binary releases and Docker image builds into a single workflow, with multi-arch Docker support (linux/amd64 + linux/arm64).

## Current State

- `.goreleaser.yaml` — builds binaries for 5 targets (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64). Uses `goreleaser-cross` Docker image for CGO cross-compilation.
- `.github/workflows/release.yaml` — triggers on `v*` tags, runs goreleaser for binary releases only.
- `.github/workflows/docker.yaml` — separate workflow, builds Docker image for linux/amd64 only via standard Docker build.
- `Dockerfile` — multi-stage: builds Go binary + llama-server from source.
- `Makefile` — `release` and `release-dry-run` targets using goreleaser-cross.

## Changes

### 1. New `Dockerfile.goreleaser`

Goreleaser-specific Dockerfile that receives the pre-built `picobrain-mcp` binary from goreleaser and still builds `llama-server` from source.

Structure:
- **Stage 1 (llama-builder)**: Clone and build llama-server from source (per-arch via `--platform`)
- **Stage 2 (runtime)**: `debian:bookworm-slim`, install runtime deps, copy `picobrain-mcp` (from goreleaser) + `llama-server` (from stage 1)

### 2. Updated `.goreleaser.yaml`

Add `dockers` and `docker_manifests` sections:

```yaml
dockers:
  - image_templates: ["asabya/picobrain:v{{ .Version }}-amd64"]
    use: buildx
    ids: [linux-amd64]
    goarch: amd64
    dockerfile: Dockerfile.goreleaser
    build_flag_templates:
      - "--platform=linux/amd64"
      - "--pull"
      - "--label=org.opencontainers.image.created={{.Date}}"
      - "--label=org.opencontainers.image.title={{.ProjectName}}"
      - "--label=org.opencontainers.image.revision={{.FullCommit}}"
      - "--label=org.opencontainers.image.version={{.Version}}"
  - image_templates: ["asabya/picobrain:v{{ .Version }}-arm64"]
    use: buildx
    ids: [linux-arm64]
    goarch: arm64
    dockerfile: Dockerfile.goreleaser
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--pull"
      - "...(same labels)"

docker_manifests:
  - name_template: "asabya/picobrain:v{{ .Major }}.{{ .Minor }}.{{ .Patch }}{{ with .Prerelease }}-{{ . }}{{ end }}"
    image_templates:
      - asabya/picobrain:v{{ .Version }}-amd64
      - asabya/picobrain:v{{ .Version }}-arm64
  - name_template: "asabya/picobrain:latest"
    image_templates:
      - asabya/picobrain:v{{ .Version }}-amd64
      - asabya/picobrain:v{{ .Version }}-arm64
```

### 3. Merged `.github/workflows/release.yaml`

Single workflow that:
1. Sets up Go 1.25
2. Checks out code with submodules
3. Sets up QEMU + Docker Buildx
4. Configures Docker Hub credentials (`.docker-creds` + `.release-env`)
5. Runs `make release` (goreleaser handles both binaries and Docker)

Delete `.github/workflows/docker.yaml` (no longer needed).

### 4. Updated `Makefile`

The `release` and `release-dry-run` targets need Docker socket mount so goreleaser can build images. Add `-v /var/run/docker.sock:/var/run/docker.sock` (already present in current Makefile — verify it's correct).

## Files to Modify

| File | Action |
|------|--------|
| `Dockerfile.goreleaser` | Create new |
| `.goreleaser.yaml` | Add dockers + docker_manifests sections |
| `.github/workflows/release.yaml` | Add QEMU, Buildx, Docker creds setup |
| `.github/workflows/docker.yaml` | Delete |
| `Makefile` | Verify Docker socket mount (likely no change needed) |

## Verification

1. `goreleaser check` — validate config syntax
2. `make release-dry-run` — verify binary builds still work
3. Trigger on a test tag to verify full pipeline
