#!/usr/bin/env bash
# start.sh — launch static server for the demo (no build step needed).
cd "$(dirname "$0")"
PORT="${1:-8000}"
echo "Serving Transformer Inference Analyzer at http://localhost:${PORT}/"
echo "(Ctrl+C to stop)"
exec python3 -m http.server "$PORT" --bind 0.0.0.0
