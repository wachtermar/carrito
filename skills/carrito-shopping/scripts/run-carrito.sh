#!/usr/bin/env bash
set -euo pipefail

skill_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
if [ -n "${CARRITO_BIN_DIR:-}" ]; then
  binary="$CARRITO_BIN_DIR/carrito"
elif [ -n "${ALCAMPO_BIN_DIR:-}" ]; then
  binary="$ALCAMPO_BIN_DIR/carrito"
elif [ -s "$skill_dir/.carrito-bin" ]; then
  binary="$(sed -n '1p' "$skill_dir/.carrito-bin")"
else
  binary="$HOME/.local/bin/carrito"
fi
if [ ! -x "$binary" ]; then
  echo "error: carrito is not installed at $binary" >&2
  echo "run: bash \"${HERMES_SKILL_DIR:-$skill_dir}/scripts/install-carrito-cli.sh\"" >&2
  exit 127
fi
exec "$binary" "$@"
