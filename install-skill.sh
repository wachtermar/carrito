#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Install the Carrito skill and CLI for Hermes Agent.

Usage:
  ./install-skill.sh [--bin-dir DIR] [--no-cli] [--login]

Defaults:
  Skill:   ${HERMES_HOME:-$HOME/.hermes}/skills/carrito-shopping
  CLI:     ${CARRITO_BIN_DIR:-$HOME/.local/bin}/carrito
  Login:   skipped; use --login to open the local browser flow
USAGE
}

repo_root="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
skill_src="$repo_root/skills/carrito-shopping"
hermes_home="${HERMES_HOME:-$HOME/.hermes}"
bin_dir="${CARRITO_BIN_DIR:-$HOME/.local/bin}"
install_cli=1
login=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --bin-dir)
      [ "$#" -ge 2 ] || { echo "error: --bin-dir requires a directory" >&2; exit 2; }
      bin_dir="$2"
      shift 2
      ;;
    --no-cli)
      install_cli=0
      shift
      ;;
    --login)
      login=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[ -f "$skill_src/SKILL.md" ] || { echo "error: missing $skill_src/SKILL.md" >&2; exit 1; }

skill_root="$hermes_home/skills"
skill_dest="$skill_root/carrito-shopping"
mkdir -p "$skill_dest"
if command -v rsync >/dev/null 2>&1; then
  rsync -a --delete --exclude '.DS_Store' --exclude '.carrito-bin' "$skill_src/" "$skill_dest/"
else
  find "$skill_dest" -mindepth 1 -maxdepth 1 ! -name '.carrito-bin' -exec rm -rf {} +
  (cd "$skill_src" && tar cf - .) | (cd "$skill_dest" && tar xf -)
fi
printf '%s\n' "$repo_root" > "$skill_dest/.carrito-source"

# Older installers also copied the same source under a category directory,
# which made Hermes index the skill twice. Remove only duplicates installed
# from this exact checkout.
while IFS= read -r duplicate; do
  [ "$duplicate" != "$skill_dest" ] || continue
  if [ -f "$duplicate/.carrito-source" ] && [ "$(sed -n '1p' "$duplicate/.carrito-source")" = "$repo_root" ]; then
    rm -rf "$duplicate"
  else
    echo "warning: another carrito-shopping skill remains at $duplicate" >&2
  fi
done < <(find "$skill_root" -mindepth 2 -maxdepth 3 -type d -name carrito-shopping 2>/dev/null | sort)

if [ "$install_cli" -eq 1 ]; then
  CARRITO_LAUNCHER_MARKER="$skill_dest/.carrito-bin" \
    "$skill_src/scripts/install-carrito-cli.sh" "$repo_root" --bin-dir "$bin_dir"
fi

if [ "$login" -eq 1 ]; then
  "$bin_dir/carrito" login-web --if-needed --json
fi

echo "installed Hermes skill: $skill_dest"
echo "try: /carrito-shopping plan three family dinners and add the groceries to my Alcampo cart"
