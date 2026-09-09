#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_ICNS="${1:-$ROOT_DIR/dist/macos/JoeJoeProxy.icns}"
PORTAL_URL="https://aigc.hkust-gz.edu.cn/"
DIRECT_ICON_URL="${MACAPP_ICON_URL:-}"
GSTATIC_FALLBACK="https://t3.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL&url=https://aigc.hkust-gz.edu.cn&size=512"
DUCKDUCKGO_FALLBACK="https://icons.duckduckgo.com/ip3/aigc.hkust-gz.edu.cn.ico"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
SOURCE_PNG="$TMP_DIR/source.png"
ICONSET_DIR="$TMP_DIR/JoeJoeProxy.iconset"

mkdir -p "$(dirname "$OUT_ICNS")" "$ICONSET_DIR"

try_icon_url() {
  local url="$1"
  local candidate="$TMP_DIR/candidate"
  [[ -n "$url" ]] || return 1
  if ! curl -fsSL --retry 2 --connect-timeout 10 --max-time 30 \
    "$url" -o "$candidate"; then
    return 1
  fi
  if ! sips -s format png "$candidate" --out "$SOURCE_PNG" >/dev/null 2>&1; then
    return 1
  fi
  sips -g pixelWidth -g pixelHeight "$SOURCE_PNG" >/dev/null
  echo "==> Using HKUST AIGC favicon: $url"
  return 0
}

resolve_portal_url() {
  local href="$1"
  case "$href" in
    http://*|https://*) printf '%s\n' "$href" ;;
    //*) printf 'https:%s\n' "$href" ;;
    /*) printf 'https://aigc.hkust-gz.edu.cn%s\n' "$href" ;;
    *) printf 'https://aigc.hkust-gz.edu.cn/%s\n' "$href" ;;
  esac
}

extract_icon_href() {
  local html="$1"
  local rel="$2"
  local href
  href="$(sed -nE "s|.*<link[^>]*rel=\"[^\"]*${rel}[^\"]*\"[^>]*href=\"([^\"]+)\"[^>]*>.*|\\1|Ip" "$html" | head -n 1)"
  if [[ -z "$href" ]]; then
    href="$(sed -nE "s|.*<link[^>]*href=\"([^\"]+)\"[^>]*rel=\"[^\"]*${rel}[^\"]*\"[^>]*>.*|\\1|Ip" "$html" | head -n 1)"
  fi
  printf '%s\n' "$href"
}

fetch_icon() {
  if [[ -n "$DIRECT_ICON_URL" ]] && try_icon_url "$DIRECT_ICON_URL"; then
    return 0
  fi

  local portal_html="$TMP_DIR/portal.html"
  if curl -fsSL --retry 2 --connect-timeout 10 --max-time 30 \
    "$PORTAL_URL" -o "$portal_html"; then
    local href discovered_url
    for rel in apple-touch-icon icon; do
      href="$(extract_icon_href "$portal_html" "$rel")"
      if [[ -n "$href" ]]; then
        discovered_url="$(resolve_portal_url "$href")"
        if try_icon_url "$discovered_url"; then
          return 0
        fi
      fi
    done
  fi

  if try_icon_url "https://aigc.hkust-gz.edu.cn/favicon.ico"; then
    return 0
  fi
  if try_icon_url "$GSTATIC_FALLBACK"; then
    echo "==> Direct portal icon unavailable; using cached favicon"
    return 0
  fi
  if try_icon_url "$DUCKDUCKGO_FALLBACK"; then
    echo "==> Direct portal icon unavailable; using cached favicon"
    return 0
  fi

  echo "Unable to retrieve the HKUST AIGC website icon." >&2
  return 1
}

render_icon() {
  local pixels="$1"
  local output="$2"
  sips -z "$pixels" "$pixels" "$SOURCE_PNG" --out "$output" >/dev/null
}

fetch_icon

render_icon 16   "$ICONSET_DIR/icon_16x16.png"
render_icon 32   "$ICONSET_DIR/icon_16x16@2x.png"
render_icon 32   "$ICONSET_DIR/icon_32x32.png"
render_icon 64   "$ICONSET_DIR/icon_32x32@2x.png"
render_icon 128  "$ICONSET_DIR/icon_128x128.png"
render_icon 256  "$ICONSET_DIR/icon_128x128@2x.png"
render_icon 256  "$ICONSET_DIR/icon_256x256.png"
render_icon 512  "$ICONSET_DIR/icon_256x256@2x.png"
render_icon 512  "$ICONSET_DIR/icon_512x512.png"
render_icon 1024 "$ICONSET_DIR/icon_512x512@2x.png"

iconutil -c icns "$ICONSET_DIR" -o "$OUT_ICNS"
echo "==> App icon: $OUT_ICNS"
