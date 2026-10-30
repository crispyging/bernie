#!/usr/bin/env bash
# Test scripts/install.sh against a fake release server.
#
# Each test runs in its own temp dir so it never touches ~/.local/bin. The
# fake server is python3's built-in http.server: it serves a fake binary and
# a SHA256SUMS file from a temp dir; install.sh uses BERNIE_BASE_URL to
# point at it.
#
# What it checks:
#   1. dry-run prints the plan and stops before download
#   2. install downloads, verifies, installs and prints "bernie up"
#   3. checksum mismatch is rejected (the binary is not installed)
#   4. unknown flag exits non-zero
#   5. unwritable --dir prints the sudo command and exits non-zero
#
# python3 is the only requirement. The script needs it on PATH.

set -euo pipefail

cd "$(dirname "$0")/.."

readonly SCRIPT="scripts/install.sh"

# install.sh supports Linux only. The test is meaningless on other OSes.
if [ "$(uname -s)" != "Linux" ]; then
    printf 'test-install.sh: skipping on %s (install.sh supports Linux only)\n' "$(uname -s)"
    exit 0
fi

# --- helpers --------------------------------------------------------------

# Pick a free TCP port by binding to 0 and reading what the kernel chose.
pick_port() {
    python3 -c '
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
'
}

# Start a Python http.server rooted at $1. Prints the PID and the port.
start_server() {
    local dir="$1" port pid
    port=$(pick_port)
    (cd "$dir" && python3 -m http.server "$port" >/dev/null 2>&1) &
    pid=$!
    # Poll until the server actually answers (max ~5 s).
    local i=0
    while [ "$i" -lt 50 ]; do
        if curl -fsS "http://127.0.0.1:$port/releases/download/$TAG/SHA256SUMS" >/dev/null 2>&1; then
            printf '%s %s\n' "$pid" "$port"
            return 0
        fi
        sleep 0.1
        i=$((i+1))
    done
    kill "$pid" 2>/dev/null || true
    printf 'test-install.sh: server did not start on port %s\n' "$port" >&2
    return 1
}

stop_server() {
    local pid="$1"
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
}

# Build the fake release: a binary that responds to `version`, and a
# SHA256SUMS file. $1 is the release dir (the root the fake server serves).
# The layout mirrors GitHub: <root>/releases/download/<tag>/<asset>.
readonly TAG="v0.0.0-test"

release_asset_dir() {
    printf '%s/releases/download/%s\n' "$1" "$TAG"
}

build_release() {
    local dir="$1" asset_dir
    asset_dir=$(release_asset_dir "$dir")
    mkdir -p "$asset_dir"
    cat > "$asset_dir/bernie-linux-amd64" <<'EOF'
#!/bin/sh
case "$1" in
    version) printf 'bernie v0.0.0-test\n' ;;
    *) printf 'fake bernie %s\n' "$*" ;;
esac
EOF
    chmod 0755 "$asset_dir/bernie-linux-amd64"
    (cd "$asset_dir" && sha256sum bernie-linux-amd64 > SHA256SUMS)
}

# Build the same release with a corrupt binary (mismatching checksum).
build_release_corrupt() {
    local dir="$1" asset_dir
    build_release "$dir"
    asset_dir=$(release_asset_dir "$dir")
    # Append a byte to the binary after the checksum was computed.
    printf 'x' >> "$asset_dir/bernie-linux-amd64"
}

# Create a temp dir with a copy of the install script. Prints the dir path.
setup() {
    local t
    t=$(mktemp -d "${TMPDIR:-/tmp}/bernie-install-test.XXXXXX")
    mkdir -p "$t/release" "$t/bin"
    : >"$t/out"; : >"$t/rc"
    cp "$SCRIPT" "$t/install.sh"
    chmod 0755 "$t/install.sh"
    printf '%s\n' "$t"
}

teardown() {
    rm -rf "$1"
}

# Run install.sh with the env vars set. Captures stdout+stderr in $1/out
# and the exit code in $1/rc.
run_install() {
    local t="$1" url="$2" bin_dir="$3" extra="${4:-}"
    set +e
    # shellcheck disable=SC2086  # $extra is a flag list; word splitting is intentional
    BERNIE_BASE_URL="$url" BERNIE_BIN_DIR="$bin_dir" BERNIE_VERSION=v0.0.0-test \
        sh "$t/install.sh" $extra >"$t/out" 2>&1
    echo $? >"$t/rc"
    set -e
}

assert_contains() {
    local name="$1" haystack="$2" needle="$3"
    case "$haystack" in
        *"$needle"*) printf '  PASS  %s\n' "$name" ;;
        *) printf '  FAIL  %s\n     missing %q in output:\n%s\n' "$name" "$needle" "$haystack"
           exit 1 ;;
    esac
}

assert_equals() {
    local name="$1" got="$2" want="$3"
    if [ "$got" = "$want" ]; then
        printf '  PASS  %s\n' "$name"
    else
        printf '  FAIL  %s\n     want %q\n     got  %q\n' "$name" "$want" "$got"
        exit 1
    fi
}

# --- tests ----------------------------------------------------------------

echo "test 1: dry-run prints the plan and does not install"
t=$(setup)
build_release "$t/release"
read -r server_pid server_port < <(start_server "$t/release")
run_install "$t" "http://127.0.0.1:$server_port" "$t/bin" "--dry-run"
rc=$(cat "$t/rc"); out=$(cat "$t/out")
stop_server "$server_pid"
assert_equals "dry-run exit code" "$rc" "0"
assert_contains "dry-run prints target" "$out" "v0.0.0-test (amd64)"
assert_contains "dry-run prints dest" "$out" "$t/bin/bernie"
assert_contains "dry-run prints download URL" "$out" "http://127.0.0.1:$server_port/releases/download/v0.0.0-test/bernie-linux-amd64"
assert_contains "dry-run stops before download" "$out" "dry-run: stopping before download"
if [ -e "$t/bin/bernie" ]; then
    printf '  FAIL  dry-run installed a binary\n'; exit 1
fi
printf '  PASS  dry-run did not install\n'
teardown "$t"

echo "test 2: install downloads, verifies and prints bernie up"
t=$(setup)
build_release "$t/release"
read -r server_pid server_port < <(start_server "$t/release")
run_install "$t" "http://127.0.0.1:$server_port" "$t/bin" ""
rc=$(cat "$t/rc"); out=$(cat "$t/out")
stop_server "$server_pid"
assert_equals "install exit code" "$rc" "0"
assert_contains "install prints target" "$out" "v0.0.0-test (amd64)"
assert_contains "install prints next step" "$out" "bernie up"
[ -x "$t/bin/bernie" ] || { printf '  FAIL  binary not installed\n'; exit 1; }
printf '  PASS  binary is executable\n'
got=$("$t/bin/bernie" version)
assert_equals "installed binary version" "$got" "bernie v0.0.0-test"
teardown "$t"

echo "test 3: checksum mismatch is rejected"
t=$(setup)
build_release_corrupt "$t/release"
read -r server_pid server_port < <(start_server "$t/release")
run_install "$t" "http://127.0.0.1:$server_port" "$t/bin" ""
rc=$(cat "$t/rc"); out=$(cat "$t/out")
stop_server "$server_pid"
assert_equals "checksum mismatch exit code" "$rc" "1"
assert_contains "checksum mismatch message" "$out" "checksum mismatch"
if [ -e "$t/bin/bernie" ]; then
    printf '  FAIL  installed despite mismatch\n'; exit 1
fi
printf '  PASS  mismatch did not install\n'
teardown "$t"

echo "test 4: unknown flag exits non-zero"
t=$(setup)
set +e
sh "$t/install.sh" --bogus >"$t/out" 2>&1
echo $? >"$t/rc"
set -e
rc=$(cat "$t/rc"); out=$(cat "$t/out")
assert_equals "unknown flag exit code" "$rc" "2"
assert_contains "unknown flag message" "$out" "unknown flag: --bogus"
teardown "$t"

echo "test 5: unwritable --dir prints the sudo command"
t=$(setup)
build_release "$t/release"
read -r server_pid server_port < <(start_server "$t/release")
# /dev/null/foo: /dev/null is a file, not a directory. mkdir -p fails
# before any download, so we do not even need the server; we keep it
# anyway in case the script reorders checks.
set +e
BERNIE_BASE_URL="http://127.0.0.1:$server_port" BERNIE_BIN_DIR="/dev/null/foo" \
    BERNIE_VERSION=v0.0.0-test sh "$t/install.sh" >"$t/out" 2>&1
echo $? >"$t/rc"
set -e
rc=$(cat "$t/rc"); out=$(cat "$t/out")
stop_server "$server_pid"
assert_equals "unwritable dir exit code" "$rc" "1"
assert_contains "unwritable dir prints sudo hint" "$out" "rerun with sudo"
teardown "$t"

echo "test 6: dry-run --start prints the listen address and skips install"
t=$(setup)
build_release "$t/release"
read -r server_pid server_port < <(start_server "$t/release")
run_install "$t" "http://127.0.0.1:$server_port" "$t/bin" "--dry-run --start --listen 0.0.0.0:7860"
rc=$(cat "$t/rc"); out=$(cat "$t/out")
stop_server "$server_pid"
assert_equals "dry-run --start exit code" "$rc" "0"
assert_contains "dry-run --start prints listen" "$out" "listen 0.0.0.0:7860"
assert_contains "dry-run --start stops before download" "$out" "stopping before download"
assert_contains "dry-run --start mentions dashboard" "$out" "dashboard"
if [ -e "$t/bin/bernie" ]; then
    printf '  FAIL  dry-run --start installed a binary\n'; exit 1
fi
printf '  PASS  dry-run --start did not install\n'
teardown "$t"

echo "test 7: --no-lan forces loopback only"
t=$(setup)
build_release "$t/release"
read -r server_pid server_port < <(start_server "$t/release")
run_install "$t" "http://127.0.0.1:$server_port" "$t/bin" "--dry-run --start --no-lan"
rc=$(cat "$t/rc"); out=$(cat "$t/out")
stop_server "$server_pid"
assert_equals "dry-run --no-lan exit code" "$rc" "0"
assert_contains "dry-run --no-lan uses loopback" "$out" "listen 127.0.0.1:7860"
teardown "$t"

echo "test 8: unknown flag with --start exits non-zero"
t=$(setup)
set +e
sh "$t/install.sh" --start --bogus >"$t/out" 2>&1
echo $? >"$t/rc"
set -e
rc=$(cat "$t/rc"); out=$(cat "$t/out")
assert_equals "unknown flag with --start exit code" "$rc" "2"
assert_contains "unknown flag message" "$out" "unknown flag: --bogus"
teardown "$t"

echo "test 9: --listen needs a value"
t=$(setup)
set +e
sh "$t/install.sh" --listen >"$t/out" 2>&1
echo $? >"$t/rc"
set -e
rc=$(cat "$t/rc"); out=$(cat "$t/out")
assert_equals "--listen missing value exit code" "$rc" "2"
assert_contains "--listen missing value message" "$out" "unknown flag: --listen"
teardown "$t"

echo "test 10: dry-run --container prints the container plan"
t=$(setup)
build_release "$t/release"
read -r server_pid server_port < <(start_server "$t/release")
run_install "$t" "http://127.0.0.1:$server_port" "$t/bin" "--dry-run --container --container-dir $t/container"
rc=$(cat "$t/rc"); out=$(cat "$t/out")
stop_server "$server_pid"
assert_equals "dry-run --container exit code" "$rc" "0"
assert_contains "dry-run --container prints the dir" "$out" "$t/container"
assert_contains "dry-run --container stops before download" "$out" "stopping before download"
assert_contains "dry-run --container mentions the compose file" "$out" "compose file"
if [ -e "$t/container/.env" ]; then
    printf '  FAIL  dry-run --container wrote an .env\n'; exit 1
fi
printf '  PASS  dry-run --container wrote nothing\n'
teardown "$t"

echo "test 11: --container-dir needs a value"
t=$(setup)
set +e
sh "$t/install.sh" --container-dir >"$t/out" 2>&1
echo $? >"$t/rc"
set -e
rc=$(cat "$t/rc"); out=$(cat "$t/out")
assert_equals "--container-dir missing value exit code" "$rc" "2"
assert_contains "--container-dir missing value message" "$out" "unknown flag: --container-dir"
teardown "$t"

echo "all install tests passed"
