# Unified Release Pipeline Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Merge goreleaser binary releases and Docker image builds into a single unified workflow with multi-arch Docker support (linux/amd64 + linux/arm64).

**Architecture:** Create a goreleaser-specific Dockerfile that receives pre-built binaries from goreleaser and builds llama-server from source. Add Docker build and manifest sections to `.goreleaser.yaml`. Merge the two GitHub Actions workflows into one that sets up QEMU + Buildx and runs goreleaser.

**Tech Stack:** GoReleaser, Docker Buildx, QEMU, GitHub Actions, Docker Hub

---

### Task 1: Create `Dockerfile.goreleaser`

**Files:**
- Create: `Dockerfile.goreleaser`
- Reference: `Dockerfile` (existing multi-stage build for llama-server patterns)

**Step 1: Write the Dockerfile.goreleaser**

```dockerfile
# Build llama-server from source
FROM --platform=$TARGETPLATFORM debian:bookworm AS llama-builder

ARG LLAMA_CPP_REF=master
ARG TARGETPLATFORM

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    cmake \
    curl \
    git \
    libcurl4-openssl-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
RUN git clone --depth 1 --branch "${LLAMA_CPP_REF}" https://github.com/ggml-org/llama.cpp .
RUN cmake -S . -B build \
    -DBUILD_SHARED_LIBS=OFF \
    -DLLAMA_BUILD_SERVER=ON \
    -DLLAMA_BUILD_TESTS=OFF \
    -DLLAMA_BUILD_EXAMPLES=OFF
RUN cmake --build build --config Release --target llama-server -j$(nproc)

# Runtime stage
FROM --platform=$TARGETPLATFORM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libcurl4 \
    libgcc-s1 \
    libstdc++6 \
    libsqlite3-0 \
    libgomp1 \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /data/models

# picobrain-mcp binary is injected by goreleaser
COPY picobrain-mcp /usr/local/bin/picobrain-mcp
COPY --from=llama-builder /src/build/bin/llama-server /usr/local/bin/llama-server

VOLUME ["/data"]

ENTRYPOINT ["picobrain-mcp"]
```

**Step 2: Verify the Dockerfile builds locally**

Run: `docker build -f Dockerfile.goreleaser --platform linux/amd64 -t picobrain-test .`
Note: This will fail without the `picobrain-mcp` binary present, but we can verify syntax.

**Step 3: Commit**

```bash
git add Dockerfile.goreleaser
git commit -m "build: add goreleaser-specific Dockerfile with llama-server build"
```

### Task 2: Update `.goreleaser.yaml` with Docker builds

**Files:**
- Modify: `.goreleaser.yaml`

**Step 1: Add `dockers` section after the `archives` section**

Add this block after line 105 (after the `archives` section):

```yaml
dockers:
  - image_templates:
      - "asabya/picobrain:v{{ .Version }}-amd64"
    use: buildx
    ids:
      - linux-amd64
    goarch: amd64
    dockerfile: Dockerfile.goreleaser
    build_flag_templates:
      - "--platform=linux/amd64"
      - "--pull"
      - "--label=org.opencontainers.image.created={{.Date}}"
      - "--label=org.opencontainers.image.title={{.ProjectName}}"
      - "--label=org.opencontainers.image.revision={{.FullCommit}}"
      - "--label=org.opencontainers.image.version={{.Version}}"
  - image_templates:
      - "asabya/picobrain:v{{ .Version }}-arm64"
    use: buildx
    ids:
      - linux-arm64
    goarch: arm64
    dockerfile: Dockerfile.goreleaser
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--pull"
      - "--label=org.opencontainers.image.created={{.Date}}"
      - "--label=org.opencontainers.image.title={{.ProjectName}}"
      - "--label=org.opencontainers.image.revision={{.FullCommit}}"
      - "--label=org.opencontainers.image.version={{.Version}}"
```

**Step 2: Add `docker_manifests` section after `dockers`**

```yaml
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

**Step 3: Validate goreleaser config syntax**

Run: `goreleaser check` (or `goreleaser check --config .goreleaser.yaml`)
Expected: Valid config

**Step 4: Commit**

```bash
git add .goreleaser.yaml
git commit -m "build: add Docker image builds and multi-arch manifests to goreleaser"
```

### Task 3: Update `.github/workflows/release.yaml` for unified workflow

**Files:**
- Modify: `.github/workflows/release.yaml`
- Reference: FaVe `.github/workflows/release.yaml` (QEMU + Buildx setup pattern)

**Step 1: Rewrite the release workflow**

```yaml
name: Release

defaults:
  run:
    shell: bash

on:
  push:
    branches-ignore:
      - '**'
    tags:
      - 'v*.*.*'

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0
          submodules: 'true'

      - name: Fetch all tags
        run: git fetch --force --tags

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: setup release environment
        run: |-
          echo '${{secrets.DOCKERHUB_USERNAME}}:${{secrets.DOCKERHUB_TOKEN}}:docker.io' > .docker-creds
          echo 'DOCKER_CREDS_FILE=.docker-creds'                                        > .release-env
          echo 'GITHUB_TOKEN=${{secrets.GITHUB_TOKEN}}'                                >> .release-env

      - name: release dry run
        run: make release-dry-run

      - name: release publish
        run: |-
          sudo rm -rf dist
          make release
```

**Step 2: Commit**

```bash
git add .github/workflows/release.yaml
git commit -m "ci: merge Docker builds into release workflow with QEMU/Buildx support"
```

### Task 4: Delete `.github/workflows/docker.yaml`

**Files:**
- Delete: `.github/workflows/docker.yaml`

**Step 1: Remove the now-redundant Docker workflow**

```bash
git rm .github/workflows/docker.yaml
```

**Step 2: Commit**

```bash
git commit -m "ci: remove standalone Docker workflow (now handled by goreleaser)"
```

### Task 5: Verify Makefile Docker socket mount

**Files:**
- Read: `Makefile` (no changes expected)

**Step 1: Verify Docker socket is mounted in release targets**

Check that `-v /var/run/docker.sock:/var/run/docker.sock` is present in both `release-dry-run` and `release` targets.

Current Makefile already has this on lines 18 and 37 — no changes needed.

**Step 2: Verify release-ddry-run still works**

Run: `make release-dry-run`
Expected: Builds succeed for all targets

### Task 6: Validate full configuration

**Step 1: Run goreleaser check**

Run: `goreleaser check`
Expected: `your config is valid`

**Step 2: Run a snapshot build to verify everything**

Run: `goreleaser release --snapshot --clean`
Expected: All 5 binary builds complete, Docker images built locally

**Step 3: Commit any fixes if needed**

```bash
git add -A
git commit -m "build: fix goreleaser config validation issues"
```
