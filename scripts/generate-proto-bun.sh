#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

PROTO_SRC="$ROOT_DIR/proto/ws.proto"
OUT_DIR="$ROOT_DIR/example/web/proto/wsproto"
WEB_DIR="$ROOT_DIR/example/web"

detect_pkg_manager() {
  if command -v apt-get >/dev/null 2>&1; then
    echo "apt"
  elif command -v dnf >/dev/null 2>&1; then
    echo "dnf"
  elif command -v yum >/dev/null 2>&1; then
    echo "yum"
  elif command -v pacman >/dev/null 2>&1; then
    echo "pacman"
  elif command -v zypper >/dev/null 2>&1; then
    echo "zypper"
  elif command -v apk >/dev/null 2>&1; then
    echo "apk"
  elif command -v brew >/dev/null 2>&1; then
    echo "brew"
  else
    echo "unknown"
  fi
}

install_protoc() {
  pkg_mgr="$(detect_pkg_manager)"
  echo "Attempting to install protoc via '$pkg_mgr'..."
  case "$pkg_mgr" in
    apt)
      sudo apt-get update && sudo apt-get install -y protobuf-compiler
      ;;
    dnf)
      sudo dnf install -y protobuf-compiler
      ;;
    yum)
      sudo yum install -y protobuf-compiler
      ;;
    pacman)
      sudo pacman -Sy --noconfirm protobuf
      ;;
    zypper)
      sudo zypper install -y protobuf-devel
      ;;
    apk)
      sudo apk add --no-cache protobuf
      ;;
    brew)
      brew install protobuf
      ;;
    *)
      echo "Error: could not detect a supported package manager."
      echo "Please install 'protoc' manually for your distribution:"
      echo "  - Debian/Ubuntu : sudo apt-get install -y protobuf-compiler"
      echo "  - Fedora/RHEL   : sudo dnf install -y protobuf-compiler"
      echo "  - CentOS/older  : sudo yum install -y protobuf-compiler"
      echo "  - Arch          : sudo pacman -S protobuf"
      echo "  - openSUSE      : sudo zypper install protobuf-devel"
      echo "  - Alpine        : sudo apk add protobuf"
      echo "  - macOS         : brew install protobuf"
      exit 1
      ;;
  esac
}

if ! command -v protoc >/dev/null 2>&1; then
  echo "'protoc' is not installed or not in PATH."
  install_protoc
fi

if ! command -v protoc >/dev/null 2>&1; then
  echo "Error: protoc installation failed or is still not in PATH."
  exit 1
fi

if [ ! -d "$WEB_DIR/node_modules" ]; then
  echo "Error: node_modules not found in $WEB_DIR"
  echo "Run 'bun install' in $WEB_DIR first."
  exit 1
fi

TS_PROTO_PLUGIN="$WEB_DIR/node_modules/.bin/protoc-gen-ts_proto"

if [ ! -x "$TS_PROTO_PLUGIN" ]; then
  echo "'ts-proto' plugin not found, installing it now..."
  (cd "$WEB_DIR" && bun add -d ts-proto)
fi

if [ ! -x "$TS_PROTO_PLUGIN" ]; then
  echo "Error: ts-proto plugin still not found at $TS_PROTO_PLUGIN"
  exit 1
fi

mkdir -p "$OUT_DIR"

echo "Generating TypeScript code from $PROTO_SRC..."
protoc \
  --plugin="protoc-gen-ts_proto=$TS_PROTO_PLUGIN" \
  --proto_path="$ROOT_DIR/proto" \
  --ts_proto_out="$OUT_DIR" \
  --ts_proto_opt=esModuleInterop=true \
  --ts_proto_opt=outputServices=false \
  --ts_proto_opt=useOptionals=messages \
  --ts_proto_opt=oneof=unions \
  --ts_proto_opt=env=node \
  "$PROTO_SRC"

echo "Successfully generated TypeScript Protobuf code in $OUT_DIR"