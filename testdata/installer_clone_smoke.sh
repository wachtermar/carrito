#!/usr/bin/env bash
set -euo pipefail

repo_root="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
test_root="$(mktemp -d)"
cleanup() {
	chmod -R u+w "$test_root" 2>/dev/null || true
  rm -rf "$test_root"
}
trap cleanup EXIT

script_dir="$test_root/installed/carrito-shopping/scripts"
fake_bin="$test_root/fake-bin"
mkdir -p "$script_dir" "$test_root/work" "$test_root/clones" "$test_root/home" "$fake_bin"
cp "$repo_root/skills/carrito-shopping/scripts/install-carrito-cli.sh" "$script_dir/"
cp "$repo_root/skills/carrito-shopping/scripts/run-carrito.sh" "$script_dir/"
printf '%s\n' "$test_root/moved-or-deleted-checkout" >"$test_root/installed/carrito-shopping/.carrito-source"
cat >"$fake_bin/go" <<'FAKE_GO'
#!/usr/bin/env bash
set -euo pipefail
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ] && [ "$#" -ge 2 ]; then
    output="$2"
    shift 2
    continue
  fi
  shift
done
if [ -n "$output" ]; then
  printf '#!/usr/bin/env bash\nexit 0\n' >"$output"
  chmod +x "$output"
fi
FAKE_GO
chmod +x "$fake_bin/go"

(
  cd "$test_root/work"
  HOME="$test_root/home" \
    PATH="$fake_bin:$PATH" \
    TMPDIR="$test_root/clones" \
    CARRITO_CLI_REPO="$repo_root" \
    CARRITO_BIN_DIR="$test_root/bin" \
    bash "$script_dir/install-carrito-cli.sh" >/dev/null
)

if [ ! -x "$test_root/bin/carrito" ]; then
  echo "installer clone smoke: binary was not installed" >&2
  exit 1
fi
HOME="$test_root/home" PATH="$fake_bin:/usr/bin:/bin" \
  HERMES_SKILL_DIR="$test_root/installed/carrito-shopping" \
  bash "$script_dir/run-carrito.sh" --help
if find "$test_root/clones" -mindepth 1 -print -quit | grep -q .; then
  echo "installer clone smoke: temporary clone was not removed" >&2
  exit 1
fi

HOME="$test_root/top-home" \
  HERMES_HOME="$test_root/top-hermes" \
  PATH="$fake_bin:$PATH" \
  bash "$repo_root/install-skill.sh" --bin-dir "$test_root/custom-bin" >/dev/null
HOME="$test_root/top-home" \
  PATH="$fake_bin:/usr/bin:/bin" \
  HERMES_SKILL_DIR="$test_root/top-hermes/skills/carrito-shopping" \
  bash "$test_root/top-hermes/skills/carrito-shopping/scripts/run-carrito.sh" --help
