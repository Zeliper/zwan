#!/usr/bin/env bash
# Build all zwan release artifacts into dist/.
#
#   scripts/build-release.sh 0.1.1
#
# Produces: dist/zwan-setup.exe (Windows installer: app + client engine/Wintun + server service),
#           dist/zwan-server-{windows-amd64.exe,windows-arm64.exe,linux-amd64,linux-arm64},
#           dist/SHA256SUMS.txt
set -euo pipefail

VER="${1:-0.0.0-dev}"
LD="-X github.com/Zeliper/zwan/shared.Version=${VER}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export PATH="/c/Program Files/Go/bin:${HOME}/go/bin:${PATH}"
MAKENSIS="/c/Program Files (x86)/NSIS/makensis.exe"

mkdir -p installer/stage dist

if [ ! -f bin/wintun.dll ]; then
  echo "ERROR: bin/wintun.dll not found — download the amd64 wintun.dll from wintun.net" >&2
  exit 1
fi

# The desktop app keeps running in the tray after its window is closed, and a
# running copy holds installer/stage/zwan.exe open. Windows then refuses the
# copy, and the build stops with an error about a busy device that says nothing
# about what to close.
# (Two "IMAGENAME eq" filters would be ANDed and match nothing, so the whole
# list is taken and filtered here. zwan-server.exe and zwan-service.exe are not
# matched: the anchored names are exact.)
if command -v tasklist >/dev/null 2>&1; then
  RUNNING="$(tasklist //NH 2>/dev/null | grep -iE '^(zwan|gui)\.exe' || true)"
  if [ -n "$RUNNING" ]; then
    echo "ERROR: the desktop app is running and holds the files this build replaces." >&2
    echo "       Quit it from the tray (closing the window is not enough), or:" >&2
    echo "         powershell -Command \"Get-Process zwan,gui -EA SilentlyContinue | Stop-Process -Force\"" >&2
    exit 1
  fi
fi

# Clear what a previous run produced. Without this a build that fails part way
# leaves the last version's artifacts sitting in dist/, looking exactly like the
# ones that were meant to be built now.
rm -f dist/zwan-setup.exe dist/zwan-server-* dist/SHA256SUMS.txt
rm -f installer/stage/zwan.exe installer/stage/zwan-service.exe installer/stage/zwan-server.exe

echo "== services (windows/amd64, for the installer) =="
go build -ldflags "$LD" -o installer/stage/zwan-service.exe ./cmd/zwan-service
go build -ldflags "$LD" -o installer/stage/zwan-server.exe  ./cmd/zwan-server
cp bin/wintun.dll installer/stage/wintun.dll

echo "== server (matrix: windows/linux x amd64/arm64) =="
GOOS=windows GOARCH=amd64 go build -ldflags "$LD" -o dist/zwan-server-windows-amd64.exe ./cmd/zwan-server
GOOS=windows GOARCH=arm64 go build -ldflags "$LD" -o dist/zwan-server-windows-arm64.exe ./cmd/zwan-server
GOOS=linux   GOARCH=amd64 go build -ldflags "$LD" -o dist/zwan-server-linux-amd64 ./cmd/zwan-server
GOOS=linux   GOARCH=arm64 go build -ldflags "$LD" -o dist/zwan-server-linux-arm64 ./cmd/zwan-server

echo "== gui (wails, windows/amd64) =="
( cd gui && wails build -ldflags "$LD" )
cp gui/build/bin/gui.exe installer/stage/zwan.exe

echo "== installer (nsis) =="
( cd installer && "$MAKENSIS" -DVERSION="$VER" zwan.nsi )

echo "== checksums =="
( cd dist && sha256sum zwan-setup.exe \
    zwan-server-windows-amd64.exe zwan-server-windows-arm64.exe \
    zwan-server-linux-amd64 zwan-server-linux-arm64 > SHA256SUMS.txt )

echo "== done =="
ls -la dist/
