#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Build and install the alcampo CLI.

Usage:
  install-alcampo-cli.sh [SOURCE] [--bin-dir DIR]

SOURCE may be:
  - a local alcampo-cli directory
  - a repository checkout containing alcampo-cli/
  - a Git URL to clone temporarily

Environment:
  ALCAMPO_CLI_SOURCE provides SOURCE when no argument is passed.
  ALCAMPO_BIN_DIR overrides the default "$HOME/.local/bin".
  ALCAMPO_INSTALL_RUN_TESTS=1 runs `go test ./...` before building.
USAGE
}

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
skill_dir="$(CDPATH= cd -- "$script_dir/.." && pwd)"
bin_dir="${ALCAMPO_BIN_DIR:-$HOME/.local/bin}"
source_arg="${ALCAMPO_CLI_SOURCE:-}"
tmp_dir=""

cleanup() {
  if [ -n "$tmp_dir" ] && [ -d "$tmp_dir" ]; then
    rm -rf "$tmp_dir"
  fi
}
trap cleanup EXIT

while [ "$#" -gt 0 ]; do
  case "$1" in
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
    -*)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [ -n "$source_arg" ]; then
        echo "error: only one SOURCE may be provided" >&2
        exit 2
      fi
      source_arg="$1"
      shift
      ;;
  esac
done

is_cli_source() {
  [ -f "$1/go.mod" ] && [ -d "$1/cmd/alcampo" ]
}

resolve_local_source() {
  for candidate in \
    "$source_arg" \
    "$skill_dir/../../alcampo-cli" \
    "$PWD/alcampo-cli" \
    "$PWD"; do
    if [ -n "$candidate" ] && [ -d "$candidate" ]; then
      candidate="$(CDPATH= cd -- "$candidate" && pwd)"
      if is_cli_source "$candidate"; then
        printf '%s\n' "$candidate"
        return 0
      fi
      if [ -d "$candidate/alcampo-cli" ] && is_cli_source "$candidate/alcampo-cli"; then
        printf '%s\n' "$candidate/alcampo-cli"
        return 0
      fi
    fi
  done
  return 1
}

clone_source() {
  url="$1"
  if ! command -v git >/dev/null 2>&1; then
    echo "error: git is required to clone $url" >&2
    exit 1
  fi
  tmp_dir="$(mktemp -d)"
  git clone --depth 1 "$url" "$tmp_dir/repo" >/dev/null
  if is_cli_source "$tmp_dir/repo"; then
    printf '%s\n' "$tmp_dir/repo"
    return 0
  fi
  if [ -d "$tmp_dir/repo/alcampo-cli" ] && is_cli_source "$tmp_dir/repo/alcampo-cli"; then
    printf '%s\n' "$tmp_dir/repo/alcampo-cli"
    return 0
  fi
  echo "error: cloned repository does not contain alcampo-cli source" >&2
  exit 1
}

if ! command -v go >/dev/null 2>&1; then
  echo "error: Go is required to build alcampo. Install Go, then rerun this script." >&2
  exit 1
fi

source_dir=""
if [ -n "$source_arg" ]; then
  case "$source_arg" in
    http://*|https://*|git@*|ssh://*)
      source_dir="$(clone_source "$source_arg")"
      ;;
  esac
fi

if [ -z "$source_dir" ]; then
  if ! source_dir="$(resolve_local_source)"; then
    cat >&2 <<'EOF'
error: could not find alcampo-cli source.

Run this script from the repository root, pass the source path explicitly, or set:
  ALCAMPO_CLI_SOURCE=/path/to/alcampo-cli
EOF
    exit 1
  fi
fi

mkdir -p "$bin_dir"

(
  cd "$source_dir"
  if [ "${ALCAMPO_INSTALL_RUN_TESTS:-0}" = "1" ]; then
    go test ./...
  fi
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$bin_dir/alcampo" ./cmd/alcampo
)

"$bin_dir/alcampo" --help >/dev/null

echo "installed alcampo: $bin_dir/alcampo"
