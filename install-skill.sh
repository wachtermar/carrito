#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Install the Alcampo Shopping skill locally.

Usage:
  ./install-skill.sh [--all|--hermes|--openclaw] [--no-cli] [--bin-dir DIR]

Defaults:
  --all
  --bin-dir "$HOME/.local/bin"

Environment:
  ALCAMPO_BIN_DIR overrides the default binary directory.
  ALCAMPO_INSTALL_RUN_TESTS=1 runs Go tests before building alcampo.
USAGE
}

repo_root="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
skill_src="$repo_root/skills/alcampo-shopping"
bin_dir="${ALCAMPO_BIN_DIR:-$HOME/.local/bin}"
install_cli=1
targets=()

while [ "$#" -gt 0 ]; do
  case "$1" in
    --all)
      targets=(hermes openclaw)
      shift
      ;;
    --hermes)
      targets+=(hermes)
      shift
      ;;
    --openclaw)
      targets+=(openclaw)
      shift
      ;;
    --no-cli)
      install_cli=0
      shift
      ;;
    --bin-dir)
      if [ "$#" -lt 2 ]; then
        echo "error: --bin-dir requires a directory" >&2
        exit 2
      fi
      bin_dir="$2"
      shift 2
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

if [ "${#targets[@]}" -eq 0 ]; then
  targets=(hermes openclaw)
fi

if [ ! -f "$skill_src/SKILL.md" ]; then
  echo "error: skill source not found at $skill_src" >&2
  exit 1
fi

copy_skill() {
  src="$1"
  dest="$2"
  mkdir -p "$dest"
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --exclude '.DS_Store' "$src/" "$dest/"
  else
    (cd "$src" && tar cf - .) | (cd "$dest" && tar xf -)
  fi
  echo "installed skill: $dest"
}

for target in "${targets[@]}"; do
  case "$target" in
    hermes)
      copy_skill "$skill_src" "$HOME/.hermes/skills/alcampo-shopping"
      ;;
    openclaw)
      copy_skill "$skill_src" "$HOME/.openclaw/skills/alcampo-shopping"
      ;;
  esac
done

if [ "$install_cli" -eq 1 ]; then
  "$skill_src/scripts/install-alcampo-cli.sh" "$repo_root/alcampo-cli" --bin-dir "$bin_dir"
fi

cat <<EOF

Done.

Try the skill in Hermes with:
  /alcampo-shopping run a 3 day dinner plan for 2 people, include low-stock staples, shop it with balanced value, receive delivered items into pantry, then remember what we liked after cooking

Try the skill in OpenClaw with:
  /skill alcampo-shopping run dinners, use what is already in my fridge, include low-stock staples, show offers/images/quantities, prepare an Alcampo basket, receive delivered items into pantry, and keep pantry/history updated

If '$bin_dir' is not on PATH, add it before starting Hermes/OpenClaw:
  export PATH="$bin_dir:\$PATH"
EOF
