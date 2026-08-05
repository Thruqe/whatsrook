#!/usr/bin/env bash
set -euo pipefail

# WhatsRook Patch Application Script
# Ensures sqlstore table definitions use clean names (no prefixing) and custom schemas are intact.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SQLSTORE_DIR="${ROOT_DIR}/store/sqlstore"

echo "==> Applying WhatsRook custom patches to store/sqlstore..."

# 1. Ensure table names do not use 'whatsmeow_' prefixing in SQL migration files
if [ -d "${SQLSTORE_DIR}/upgrades" ]; then
  find "${SQLSTORE_DIR}/upgrades" -type f -name "*.sql" -exec sed -i 's/whatsmeow_//g' {} +
fi

# 2. Ensure Go source files in sqlstore do not use prefixing
find "${SQLSTORE_DIR}" -type f -name "*.go" -exec sed -i 's/whatsmeow_//g' {} +

echo "==> WhatsRook patches applied successfully!"
