#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARCH="${ARCH:-$(uname -m)}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/dist/macos/$ARCH}"
APP_NAME="SchoolSub2API"
APP_DIR="$OUT_DIR/$APP_NAME.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

case "$ARCH" in
  arm64)
    GOARCH="arm64"
    ;;
  x86_64)
    GOARCH="amd64"
    ;;
  *)
    echo "Unsupported macOS architecture: $ARCH" >&2
    exit 1
    ;;
esac

rm -rf "$OUT_DIR"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

export MACOSX_DEPLOYMENT_TARGET="13.0"

echo "==> Building ds2api for darwin/$GOARCH"
(
  cd "$ROOT_DIR"
  CGO_ENABLED=0 GOOS=darwin GOARCH="$GOARCH" \
    go build -trimpath -ldflags="-s -w" \
    -o "$RESOURCES_DIR/ds2api" ./cmd/ds2api
)
chmod 755 "$RESOURCES_DIR/ds2api"

echo "==> Building SwiftUI launcher for $ARCH"
swift build \
  --package-path "$ROOT_DIR/macapp" \
  --configuration release \
  --arch "$ARCH"

SWIFT_BIN_DIR="$(swift build \
  --package-path "$ROOT_DIR/macapp" \
  --configuration release \
  --arch "$ARCH" \
  --show-bin-path)"
cp "$SWIFT_BIN_DIR/SchoolSub2APIMac" "$MACOS_DIR/SchoolSub2APIMac"
chmod 755 "$MACOS_DIR/SchoolSub2APIMac"
cp "$ROOT_DIR/macapp/Info.plist" "$CONTENTS_DIR/Info.plist"

if command -v codesign >/dev/null 2>&1; then
  echo "==> Applying ad-hoc signature"
  codesign --force --deep --sign - "$APP_DIR"
fi

ZIP_PATH="$OUT_DIR/$APP_NAME-macos-$ARCH.zip"
echo "==> Creating $ZIP_PATH"
ditto -c -k --sequesterRsrc --keepParent "$APP_DIR" "$ZIP_PATH"

echo
printf 'Built app: %s\n' "$APP_DIR"
printf 'Zip:       %s\n' "$ZIP_PATH"
