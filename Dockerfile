# Build stage
FROM --platform=linux/amd64 golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc6-dev \
    libsqlite3-dev \
    build-essential

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o picobrain-mcp ./cmd/picobrain-mcp

# Download pre-built llama-server and libraries
ARG LLAMA_CPP_VERSION=8606

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    tar \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fSL -o /tmp/llama.tar.gz \
      "https://github.com/ggml-org/llama.cpp/releases/download/b${LLAMA_CPP_VERSION}/llama-b${LLAMA_CPP_VERSION}-bin-ubuntu-x64.tar.gz" && \
    mkdir -p /opt/llama && \
    tar xzf /tmp/llama.tar.gz --strip-components=1 -C /opt/llama/ && \
    chmod +x /opt/llama/* && \
    rm /tmp/llama.tar.gz

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

# Copy all llama binaries and libs together (they depend on each other)
COPY --from=builder /opt/llama/ /opt/llama/

# Make llama-server discoverable and register libs
RUN ln -s /opt/llama/llama-server /usr/local/bin/llama-server && \
    echo "/opt/llama" > /etc/ld.so.conf.d/llama.conf && \
    ldconfig

ENV LD_LIBRARY_PATH=/opt/llama

VOLUME ["/data"]

ENTRYPOINT ["picobrain-mcp"]
