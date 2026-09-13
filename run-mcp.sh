#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY="$SCRIPT_DIR/bin/nexus"

# Auto-build if binary missing or sources newer
if [[ ! -f "$BINARY" ]] || find "$SCRIPT_DIR" -name '*.go' -newer "$BINARY" -print -quit | grep -q .; then
    echo "nexus: building..." >&2
    make -C "$SCRIPT_DIR" build >&2
fi

exec "$BINARY" serve "$@"
