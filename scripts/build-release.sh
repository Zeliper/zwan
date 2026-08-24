#!/usr/bin/env bash
# Build all zwan release artifacts into dist/.
#
#   scripts/build-release.sh 0.1.0
#
# Produces: dist/zwan-setup.exe (Windows client installer: GUI + service + Wintun),
#           dist/zwan-server-windows-amd64.exe, dist/zwan-server-linux-amd64,
#           dist/SHA256SUMS.txt
set -euo pipefail

VER="${1:-0.0.0-dev}"
LD="-X github.com/Zeliper/zwan/shared.Version=${VER}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export PATH="/c/Program Files/Go/bin:${HOME}/go/bin:${PATH}"
MAKENSIS="/c/Program Files (x86)/NSIS/makensis.exe"

mkdir -p installer/stage dist

# Wintun redistributable: reuse bin/wintun.dll if present (fetch the official
# build from https://www.wintun.net/ for a clean release).
if [ ! -f bin/wintun.dll ]; then
  echo "ERROR: bin/wintun.dll not found — download the amd64 wintun.dll from wintun.net" >&2
  exit 1
fi

echo "== service (windows) =="
go build -ldflags "$LD" -o installer/stage/zwan-service.exe ./cmd/zwan-service

echo "== server (windows + linux) =="
go build -ldflags "$LD" -o dist/zwan-server-windows-amd64.exe ./cmd/zwan-server
GOOS=linux GOARCH=amd64 go build -ldflags "$LD" -o dist/zwan-server-linux-amd64 ./cmd/zwan-server

cp bin/wintun.dll installer/stage/wintun.dll

echo "== gui (wails) =="
( cd gui && wails build -ldflags "$LD" )
cp gui/build/bin/gui.exe installer/stage/zwan.exe

echo "== installer (nsis) =="
( cd installer && "$MAKENSIS" -DVERSION="$VER" zwan.nsi )

echo "== checksums =="
( cd dist && sha256sum zwan-setup.exe zwan-server-windows-amd64.exe zwan-server-linux-amd64 > SHA256SUMS.txt )

echo "== done =="
ls -la dist/
