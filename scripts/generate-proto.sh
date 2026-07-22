#!/bin/sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

PROTO_SRC="$ROOT_DIR/proto/ws.proto"
OUT_DIR="$ROOT_DIR/proto/wsproto"

if ! command -v protoc >/dev/null 2>&1; then
  echo "Error: 'protoc' is not installed or not in PATH."
  echo "Please install protoc (Protocol Buffers compiler):"
  echo "  - Linux (Ubuntu/Debian): sudo apt-get install -y protobuf-compiler"
  echo "  - macOS: brew install protobuf"
  exit 1
fi

if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "Warning: 'protoc-gen-go' plugin is not in PATH."
  echo "Installing protoc-gen-go..."
  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

mkdir -p "$OUT_DIR"

echo "Generating Go code from $PROTO_SRC..."
protoc \
  --proto_path="$ROOT_DIR/proto" \
  --go_out="$OUT_DIR" \
  --go_opt=paths=source_relative \
  "$PROTO_SRC"

echo "Successfully generated Protobuf code in $OUT_DIR"
