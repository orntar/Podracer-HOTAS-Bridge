#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
command -v go >/dev/null
command -v mksquashfs >/dev/null
: "${APPIMAGE_RUNTIME:?Set APPIMAGE_RUNTIME to a verified x86_64 AppImage type2 runtime file}"
go test ./...
mkdir -p build/AppDir dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w -buildid=' -o build/AppDir/AppRun .
cp packaging/podracer-bridge.desktop web/icon.svg build/AppDir/
mv build/AppDir/icon.svg build/AppDir/podracer-bridge.svg
cp web/icon.svg build/AppDir/.DirIcon
cp README.md LICENSE THIRD_PARTY_NOTICES.md build/AppDir/
cp -r licenses build/AppDir/
cp BUILD_PROVENANCE.json build/AppDir/
cp VALIDATION.md build/AppDir/
chmod 755 build/AppDir/AppRun
mksquashfs build/AppDir build/payload.squashfs -noappend -all-root -root-mode 755 -comp gzip
cat "$APPIMAGE_RUNTIME" build/payload.squashfs > dist/PodracerBridge-0.1.0-x86_64.AppImage
chmod +x dist/PodracerBridge-0.1.0-x86_64.AppImage
(cd dist && sha256sum PodracerBridge-0.1.0-x86_64.AppImage > SHA256SUMS)
dist/PodracerBridge-0.1.0-x86_64.AppImage --appimage-extract-and-run --self-test
