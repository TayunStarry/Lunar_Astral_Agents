#!/bin/bash
# Download OmniVoice checkpoint from HuggingFace.
# Usage: ./checkpoints.sh

set -eu

DIR="checkpoints"
mkdir -p "$DIR"

HF="hf download --quiet"

dl_repo() {
    local name="$1" repo="$2"
    local target="$DIR/$name"
    if [ -d "$target" ] && [ "$(ls "$target"/*.safetensors 2>/dev/null | wc -l)" -gt 0 ]; then
        echo "[OK] $name"
        return
    fi
    echo "[Download] $name <- $repo"
    $HF "$repo" --local-dir "$target"
}

# k2-fsa/OmniVoice : LLM core (Qwen3 0.6B + audio_emb + audio_heads)
#                    audio_tokenizer/ subdir (HuBERT + DAC + RVQ + fc/fc2)
#                    tokenizer.json + chat_template.jinja
dl_repo "OmniVoice" "k2-fsa/OmniVoice"
