#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root/frontend"
npm ci
npm run build
cd "$root"
export CGO_ENABLED=0
if [[ -x "${HOME}/sdk/go/bin/go" ]]; then
  export PATH="${HOME}/sdk/go/bin:${PATH}"
fi
mkdir -p bin
go build -trimpath -ldflags "-s -w" -o bin/edugo ./cmd/server
echo "built bin/edugo"
