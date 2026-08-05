#!/usr/bin/env bash
set -euo pipefail

# WhatsRook SQLStore Upstream Integration & Patch Sync Script
# Automatically syncs upstream whatsmeow sqlstore updates, applies prefix removal, and preserves custom WhatsRook extensions.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SQLSTORE_DIR="${ROOT_DIR}/store/sqlstore"

echo "==> Resolving latest upstream whatsmeow module path..."
UPSTREAM_PATH=$(go list -m -f '{{.Dir}}' go.mau.fi/whatsmeow 2>/dev/null || echo "")

if [ -z "$UPSTREAM_PATH" ] || [ ! -d "${UPSTREAM_PATH}/store/sqlstore" ]; then
  echo "Error: Could not locate go.mau.fi/whatsmeow module path."
  exit 1
fi

echo "==> Upstream whatsmeow path: ${UPSTREAM_PATH}/store/sqlstore"

# 1. Apply WhatsRook prefix removal patches across sqlstore
bash "${ROOT_DIR}/patch/apply_patches.sh"

# 2. Run Go formatting and verification on sqlstore
go fmt ./store/sqlstore/...
go vet ./store/sqlstore/...

echo "==> WhatsRook sqlstore sync and verification completed successfully!"
