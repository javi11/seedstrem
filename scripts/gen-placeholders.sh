#!/usr/bin/env bash
# Regenerates internal/stream/assets/downloading_*.mp4 — the "still
# downloading" placeholder clips served when a stream's head/tail pieces
# are not available yet. One clip per 10% progress bucket.
# Requires macOS (Swift/AppKit renders the frame) and ffmpeg.
set -euo pipefail
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for pct in 0 10 20 30 40 50 60 70 80 90; do
  swift scripts/placeholder-frame.swift "$pct" "$tmp/frame.png"
  ffmpeg -y -loglevel error -loop 1 -framerate 24 -t 12 -i "$tmp/frame.png" \
    -f lavfi -t 12 -i "anullsrc=channel_layout=stereo:sample_rate=44100" \
    -c:v libx264 -profile:v baseline -level 3.1 -pix_fmt yuv420p \
    -preset veryslow -crf 32 -c:a aac -b:a 32k -shortest -movflags +faststart \
    "internal/stream/assets/downloading_${pct}.mp4"
  echo "downloading_${pct}.mp4 done"
done
