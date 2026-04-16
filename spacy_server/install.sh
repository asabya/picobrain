#!/usr/bin/env bash
# Install SpaCy server for picobrain's dependency parsing graph feature.
# Creates a virtualenv, installs dependencies, and downloads the model.
#
# Usage: ./install.sh
# Environment: PICOBRAIN_SPACY_DIR (default: ~/.picobrain/spacy)

set -euo pipefail

SPACY_DIR="${PICOBRAIN_SPACY_DIR:-$HOME/.picobrain/spacy}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Installing SpaCy server to: $SPACY_DIR"

# Create target directory
mkdir -p "$SPACY_DIR"

# Copy server files if running from source
if [ "$SCRIPT_DIR" != "$SPACY_DIR" ]; then
    cp "$SCRIPT_DIR/server.py" "$SPACY_DIR/server.py"
    cp "$SCRIPT_DIR/requirements.txt" "$SPACY_DIR/requirements.txt"
fi

# Create virtual environment
if [ ! -d "$SPACY_DIR/venv" ]; then
    echo "Creating virtual environment..."
    python3 -m venv "$SPACY_DIR/venv"
fi

# Install dependencies
echo "Installing Python dependencies..."
"$SPACY_DIR/venv/bin/pip" install --quiet --upgrade pip
"$SPACY_DIR/venv/bin/pip" install --quiet -r "$SPACY_DIR/requirements.txt"

# Download SpaCy model
echo "Downloading SpaCy model (en_core_web_sm)..."
"$SPACY_DIR/venv/bin/python" -m spacy download en_core_web_sm

# Try to install coref (optional)
echo "Attempting to install coreference resolution (optional)..."
"$SPACY_DIR/venv/bin/pip" install --quiet spacy-coref 2>/dev/null || echo "  Coref not available — pronoun resolution will use fallback heuristic"

echo ""
echo "Installation complete! SpaCy server is ready at: $SPACY_DIR"
echo "Start with: $SPACY_DIR/venv/bin/python -m uvicorn server:app --port 8000"