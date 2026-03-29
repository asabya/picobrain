#!/bin/bash
set -e

# Create data directory if it doesn't exist
mkdir -p "$(dirname "$0")/../data"

# Build the Docker image
docker compose build

# Start the container with HTTP transport on port 8080
docker compose up -d

echo "picobrain is running with HTTP transport at http://localhost:8080"
echo "Data persisted at ./data/"
