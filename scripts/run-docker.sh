#!/bin/bash
set -e

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DATA_DIR="$ROOT_DIR/data"

# Persist both the SQLite database and the downloaded model cache.
mkdir -p "$DATA_DIR/models"

# Build the Docker image
docker compose build --no-cache

# Start the container with HTTP transport on port 8080
docker compose up -d --force-recreate

echo "picobrain is running with HTTP transport at http://localhost:8080"
echo "Data and model cache persisted at $DATA_DIR"
