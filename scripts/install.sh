#!/bin/sh
set -eu

REPO="${TMUX_PARATOR_REPO:-sschmerda/tmux-parator}"
VERSION="${TMUX_PARATOR_VERSION:-latest}"
INSTALL_DIR="${TMUX_PARATOR_INSTALL_DIR:-$HOME/.local/bin}"
BINARY_NAME="tmux-parator"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "tmux-parator: missing required command: $1" >&2
    exit 1
  fi
}

detect_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux) echo "linux" ;;
    *)
      echo "tmux-parator: unsupported OS: $(uname -s)" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) echo "amd64" ;;
    arm64 | aarch64) echo "arm64" ;;
    *)
      echo "tmux-parator: unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

download() {
  url="$1"
  output="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$output"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$output" "$url"
    return
  fi
  echo "tmux-parator: missing required command: curl or wget" >&2
  exit 1
}

verify_checksum() {
  checksum_file="$1"
  archive="$2"
  expected="$(grep "  ${archive}$" "$checksum_file" || true)"
  if [ -z "$expected" ]; then
    echo "tmux-parator: checksum not found for $archive" >&2
    exit 1
  fi
  if command -v shasum >/dev/null 2>&1; then
    (cd "$tmpdir" && printf '%s\n' "$expected" | shasum -a 256 -c >/dev/null)
    return
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$tmpdir" && printf '%s\n' "$expected" | sha256sum -c >/dev/null)
    return
  fi
  echo "tmux-parator: missing required command: sha256sum or shasum" >&2
  exit 1
}

need uname
need tar
need mktemp
need mkdir
need install
need grep

os="$(detect_os)"
arch="$(detect_arch)"
archive="${BINARY_NAME}_${os}_${arch}.tar.gz"

if [ "$VERSION" = "latest" ]; then
  url="https://github.com/${REPO}/releases/latest/download/${archive}"
  checksum_url="https://github.com/${REPO}/releases/latest/download/checksums.txt"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${archive}"
  checksum_url="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
fi

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

download "$url" "$tmpdir/$archive"
download "$checksum_url" "$tmpdir/checksums.txt"
verify_checksum "$tmpdir/checksums.txt" "$archive"
tar -xzf "$tmpdir/$archive" -C "$tmpdir"

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmpdir/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"

echo "tmux-parator installed to $INSTALL_DIR/$BINARY_NAME"
