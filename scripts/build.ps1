$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location (Join-Path $root "frontend")
npm ci
npm run build
Set-Location $root
$env:CGO_ENABLED = "0"
New-Item -ItemType Directory -Force -Path (Join-Path $root "bin") | Out-Null
go build -trimpath -ldflags "-s -w" -o (Join-Path $root "bin/edugo.exe") ./cmd/server
Write-Host "built bin/edugo.exe"
