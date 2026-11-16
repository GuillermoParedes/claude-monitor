#!/bin/sh
# claude-monitor installer for macOS, Linux and WSL.
#
#   curl -fsSL https://raw.githubusercontent.com/GuillermoParedes/claude-monitor/main/scripts/install.sh | sh
#
# Downloads the latest release for this OS/arch and runs `claude-monitor install`,
# which copies itself to ~/.local/bin, adds the Claude Code hooks/statusline and
# sets up autostart. On WSL it also installs the Windows tray app.
set -eu

REPO="GuillermoParedes/claude-monitor"
VERSION="${CLAUDE_MONITOR_VERSION:-latest}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $os (on Windows use install.ps1)" >&2; exit 1 ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

asset="claude-monitor_${os}_${arch}"
if [ "$VERSION" = "latest" ]; then
  url="https://github.com/$REPO/releases/latest/download/$asset"
  sums="https://github.com/$REPO/releases/latest/download/checksums.txt"
else
  url="https://github.com/$REPO/releases/download/$VERSION/$asset"
  sums="https://github.com/$REPO/releases/download/$VERSION/checksums.txt"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $asset ..."
curl -fsSL "$url" -o "$tmp/claude-monitor"
if curl -fsSL "$sums" -o "$tmp/checksums.txt" 2>/dev/null; then
  expected=$(grep " $asset\$" "$tmp/checksums.txt" | cut -d' ' -f1)
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$tmp/claude-monitor" | cut -d' ' -f1)
  else
    actual=$(shasum -a 256 "$tmp/claude-monitor" | cut -d' ' -f1)
  fi
  if [ -n "$expected" ] && [ "$expected" != "$actual" ]; then
    echo "Checksum mismatch for $asset" >&2
    exit 1
  fi
fi
chmod +x "$tmp/claude-monitor"

"$tmp/claude-monitor" install "$@"

case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) printf "\nNote: add ~/.local/bin to your PATH to use the claude-monitor command.\n" ;;
esac
