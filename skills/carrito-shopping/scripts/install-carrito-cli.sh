#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Build and install the carrito CLI.

Usage:
  install-carrito-cli.sh [SOURCE] [--bin-dir DIR]

SOURCE may be:
  - a local carrito checkout
  - a repository checkout containing go.mod and cmd/carrito
  - a Git URL to clone temporarily

When SOURCE is omitted, the script first tries local checkout paths and then
falls back to the public GitHub repository.

Environment:
  CARRITO_CLI_SOURCE provides SOURCE when no argument is passed.
  CARRITO_CLI_REPO overrides the fallback Git repository.
  CARRITO_BIN_DIR overrides the default "$HOME/.local/bin".
  CARRITO_INSTALL_RUN_TESTS=1 runs `go test ./...` before building.
  Legacy ALCAMPO_* install variables are still accepted.
USAGE
}

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
skill_dir="$(CDPATH= cd -- "$script_dir/.." && pwd)"
bin_dir="${CARRITO_BIN_DIR:-${ALCAMPO_BIN_DIR:-$HOME/.local/bin}}"
source_arg="${CARRITO_CLI_SOURCE:-${ALCAMPO_CLI_SOURCE:-}}"
source_marker="$skill_dir/.carrito-source"
default_repo="${CARRITO_CLI_REPO:-${ALCAMPO_CLI_REPO:-https://github.com/wachtermar/carrito.git}}"
tmp_dir=""

if [ -z "$source_arg" ] && [ -f "$source_marker" ]; then
  source_arg="$(sed -n '1p' "$source_marker")"
fi

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
  [ -f "$1/go.mod" ] && [ -d "$1/cmd/carrito" ]
}

resolve_local_source() {
  for candidate in \
    "$source_arg" \
    "$skill_dir/../.." \
    "$PWD/carrito" \
    "$PWD"; do
    if [ -n "$candidate" ] && [ -d "$candidate" ]; then
      candidate="$(CDPATH= cd -- "$candidate" && pwd)"
      if is_cli_source "$candidate"; then
        printf '%s\n' "$candidate"
        return 0
      fi
      if [ -d "$candidate/carrito" ] && is_cli_source "$candidate/carrito"; then
        printf '%s\n' "$candidate/carrito"
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
  if [ -d "$tmp_dir/repo/carrito" ] && is_cli_source "$tmp_dir/repo/carrito"; then
    printf '%s\n' "$tmp_dir/repo/carrito"
    return 0
  fi
  echo "error: cloned repository does not contain carrito source" >&2
  exit 1
}

if ! command -v go >/dev/null 2>&1; then
  echo "error: Go is required to build carrito. Install Go, then rerun this script." >&2
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
    if [ -z "$source_arg" ]; then
      echo "local carrito source was not found; cloning $default_repo" >&2
      source_dir="$(clone_source "$default_repo")"
    else
      cat >&2 <<'EOF'
error: could not find carrito source.

Run this script from the repository root, pass the source path explicitly, or set:
  CARRITO_CLI_SOURCE=/path/to/carrito
EOF
      exit 1
    fi
  fi
fi

mkdir -p "$bin_dir"

(
  cd "$source_dir"
  if [ "${CARRITO_INSTALL_RUN_TESTS:-${ALCAMPO_INSTALL_RUN_TESTS:-0}}" = "1" ]; then
    go test ./...
  fi
  commit="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  ldflags="-s -w -X github.com/wachtermar/carrito/internal/cli.Version=dev -X github.com/wachtermar/carrito/internal/cli.Commit=$commit -X github.com/wachtermar/carrito/internal/cli.Date=$date"
  CGO_ENABLED=0 go build -trimpath -ldflags="$ldflags" -o "$bin_dir/carrito" ./cmd/carrito
)

"$bin_dir/carrito" --help >/dev/null

echo "installed carrito: $bin_dir/carrito"
