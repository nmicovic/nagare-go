#!/usr/bin/env bash
set -euo pipefail

go build -ldflags="-s -w" -o nagare-go .
echo "Built: ./nagare-go ($(du -h nagare-go | cut -f1))"
