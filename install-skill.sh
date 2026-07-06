#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Install the Alcampo Shopping skill locally.

Usage:
  ./install-skill.sh [--all|--hermes|--openclaw] [--no-cli] [--login|--no-login] [--bin-dir DIR]

Defaults:
  --all
  --bin-dir "$HOME/.local/bin"
  --login

Environment:
  ALCAMPO_BIN_DIR overrides the default binary directory.
  ALCAMPO_INSTALL_RUN_TESTS=1 runs Go tests before building alcampo.
  ALCAMPO_INSTALL_LOGIN=0 skips the post-install browser login check.
USAGE
}

repo_root="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
skill_src="$repo_root/skills/alcampo-shopping"
bin_dir="${ALCAMPO_BIN_DIR:-$HOME/.local/bin}"
install_cli=1
login_after_install="${ALCAMPO_INSTALL_LOGIN:-1}"
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
    --login)
      login_after_install=1
      shift
      ;;
    --no-login)
      login_after_install=0
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
  printf '%s\n' "$repo_root/alcampo-cli" > "$dest/.alcampo-cli-source"
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

alcampo_cmd=""
if [ -x "$bin_dir/alcampo" ]; then
  alcampo_cmd="$bin_dir/alcampo"
elif command -v alcampo >/dev/null 2>&1; then
  alcampo_cmd="$(command -v alcampo)"
fi

case "$login_after_install" in
  0|false|False|FALSE|no|No|NO|off|Off|OFF)
    ;;
  *)
    if [ -n "$alcampo_cmd" ]; then
      echo "checking Alcampo login session..."
      if ! "$alcampo_cmd" login-web --if-needed --json; then
        echo "warning: Alcampo login was not completed. Run '$alcampo_cmd login-web --if-needed' when ready." >&2
      fi
    else
      echo "warning: alcampo binary not found; skipping login check." >&2
    fi
    ;;
esac

cat <<EOF

Done.

Try the skill in Hermes with:
  /alcampo-shopping run a 3 day dinner plan for 2 people, include low-stock staples, shop it with balanced value, receive delivered items into pantry, then remember what we liked after cooking

Try the skill in OpenClaw with:
  /skill alcampo-shopping run dinners, use what is already in my fridge, include low-stock staples, show offers/images/quantities, prepare an Alcampo basket, receive delivered items into pantry, and keep pantry/history updated

If '$bin_dir' is not on PATH, add it before starting Hermes/OpenClaw:
  export PATH="$bin_dir:\$PATH"

To authenticate later from Hermes Desktop or a terminal:
  alcampo login-web --if-needed
EOF
