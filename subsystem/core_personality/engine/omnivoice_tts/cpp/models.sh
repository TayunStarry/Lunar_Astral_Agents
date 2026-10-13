#!/bin/bash
# Download pre-quantized OmniVoice GGUF models from HuggingFace.
# Usage: ./models.sh

set -eu

REPO="Serveurperso/OmniVoice-GGUF"
DIR="models"
mkdir -p "$DIR"

dl() {
    local file="$1"
    if [ -f "$DIR/$file" ]; then
        echo "[OK] $file"
        return
    fi
    echo "[Download] $file"
    hf download --quiet "$REPO" "$file" --local-dir "$DIR"
}

dl "omnivoice-base-Q8_0.gguf"
dl "omnivoice-tokenizer-F32.gguf"
