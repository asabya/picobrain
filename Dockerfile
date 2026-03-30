# Build stage
FROM --platform=linux/amd64 golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc6-dev \
    libsqlite3-dev \
    build-essential \
    cmake

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o picobrain-mcp ./cmd/picobrain-mcp

# Build a recent llama-server that supports nomic-bert GGUF embeddings.
FROM --platform=linux/amd64 debian:bookworm AS llama-builder

ARG LLAMA_CPP_REF=master

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
FROM --platform=linux/amd64 debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libcurl4 \
    libgcc-s1 \
    libstdc++6 \
    libsqlite3-0 \
    libgomp1 \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /data/models

COPY --from=builder /app/picobrain-mcp /usr/local/bin/
COPY --from=llama-builder /src/build/bin/llama-server /usr/local/bin/

VOLUME ["/data"]

ENTRYPOINT ["picobrain-mcp"]
