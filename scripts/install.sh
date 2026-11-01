#!/bin/sh
# Install the latest bernie release binary and verify its checksum. When no
# release exists, build bernie from source, with Go in a folder of this user.
#
#   curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh
#
# One command for the whole path (install, then `bernie up`):
#
#   curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh -s -- --up
#
# Flags:
#   --version TAG    release tag, e.g. v0.1.0 (default: latest)
#   --dir DIR        install directory (default: ~/.local/bin;
#                    if DIR is not writable, prints the exact command
#                    for you to run with sudo and exits non-zero)
#   --start          start the dashboard service after install (one-line
#                    then browser)
#   --listen ADDR    listen address for the dashboard (default:
#                    127.0.0.1:7860; a non-loopback address is opt-in
#                    and requires a token; the URL carries ?token=...)
#   --no-lan         equivalent to --listen 127.0.0.1:7860 (the default)
#   --host           equivalent to --listen 0.0.0.0:7860: the dashboard
#                    is reachable from another machine on the network.
#                    The token is then the only protection.
#   --container      also install the dashboard container: the compose
#                    file, a .env with the token and the GPU group ids,
#                    and the container up
#   --container-dir DIR  where the container files go (default:
#                    ~/.local/share/bernie/container)
#   --up             run `bernie up` after the install: check, setup, serve,
#                    dashboard. It stops at a step that needs root and prints
#                    the command.
#   --yes            with --up, do not ask for confirmation
#   --from-source    build from source even when a release exists
#   --dry-run        print what would be done, do nothing
#   -h | --help      this help
#
# Environment (flags take precedence):
#   BERNIE_REPO      GitHub owner/repo        (default opengfx1030/bernie)
#   BERNIE_VERSION   release tag              (default: latest)
#   BERNIE_BIN_DIR   install directory        (default: ~/.local/bin)
#   BERNIE_BASE_URL  base URL (testing only; default https://github.com/$repo)
#   BERNIE_RAW_URL   raw base URL (testing only; default
#                    https://raw.githubusercontent.com/$repo)
#   BERNIE_LISTEN    listen address for --start (same as --listen)
#   BERNIE_GO_DIR    where the build keeps Go and its caches
#                    (default: ~/.local/share/bernie/go)
#   BERNIE_GO_URL    Go download base URL (testing only; default https://dl.google.com/go)
#
# The script never calls `sudo` itself. If the chosen directory is not
# writable, it prints the exact command and exits non-zero.
#
# Re-running the line updates the binary. With --start, the dashboard is
# restarted only when the installed version changed.

set -eu

usage() {
    cat <<'EOF'
Usage: install.sh [--version TAG] [--dir DIR] [--start] [--listen ADDR] [--no-lan] [--host] [--container] [--container-dir DIR] [--up] [--yes] [--from-source] [--dry-run] [-h|--help]

Install the latest bernie release binary. When no release exists, build
bernie from source, with Go in a folder of this user.

  --version TAG    release tag, e.g. v0.1.0 (default: latest)
  --dir DIR        install directory (default: ~/.local/bin)
  --start          start the dashboard service after install
  --listen ADDR    listen address for the dashboard (default 127.0.0.1:7860)
  --no-lan         force loopback only (same as the default)
  --container      install the dashboard container (compose file, .env,
                   container up)
  --container-dir DIR  where the container files go
                   (default: ~/.local/share/bernie/container)
  --up             run `bernie up` after the install
  --yes            with --up, do not ask for confirmation
  --from-source    build from source even when a release exists
  --dry-run        print what would be done; do nothing

Environment:
  BERNIE_REPO      GitHub owner/repo        (default opengfx1030/bernie)
  BERNIE_VERSION   release tag              (overrides --version)
  BERNIE_BIN_DIR   install directory        (overrides --dir)
  BERNIE_BASE_URL  base URL                 (testing only)
  BERNIE_RAW_URL   raw base URL             (testing only)
  BERNIE_LISTEN    listen address           (overrides --listen)
  BERNIE_GO_DIR    where the build keeps Go and its caches
  BERNIE_GO_URL    Go download base URL     (testing only)

After install, run:  bernie  (opens the dashboard; add --listen ADDR for the LAN)

With --start the dashboard is left running as a background service; the
script then prints the loopback URL, the LAN URLs and an ssh -L hint.

With --container the dashboard runs in a container instead: the compose
file, a .env with the token and the GPU group ids, and the container
up. The container mounts the GPU device nodes and /sys (read-only); it
does not mount the engine socket. See SECURITY.md.
EOF
}

# Defaults. The flags override the environment.
repo=${BERNIE_REPO:-opengfx1030/bernie}
version=${BERNIE_VERSION:-latest}
bin_dir=${BERNIE_BIN_DIR:-$HOME/.local/bin}
base_url=${BERNIE_BASE_URL:-https://github.com/$repo}
raw_base=${BERNIE_RAW_URL:-https://raw.githubusercontent.com/$repo}
listen=${BERNIE_LISTEN:-127.0.0.1:7860}
container_dir=${BERNIE_CONTAINER_DIR:-$HOME/.local/share/bernie/container}
go_dir=${BERNIE_GO_DIR:-$HOME/.local/share/bernie/go}
go_url=${BERNIE_GO_URL:-https://dl.google.com/go}
dry_run=0
start_after=0
container=0
run_after=0
assume_yes=0
from_source=0

while [ $# -gt 0 ]; do
    # A flag that takes a value needs one; set -u would abort on $2.
    case "$1" in
        --version|--dir|--listen|--container-dir)
            if [ $# -lt 2 ]; then
                printf 'bernie: %s needs a value\n\n' "$1" >&2
                usage >&2
                exit 2
            fi ;;
    esac
    case "$1" in
        --version) version=$2; shift 2 ;;
        --dir)     bin_dir=$2; shift 2 ;;
        --start)   start_after=1; shift ;;
        --listen)  listen=$2; shift 2 ;;
        --no-lan)  listen=127.0.0.1:7860; shift ;;
        --host)    listen=0.0.0.0:7860; shift ;;
        --container) container=1; shift ;;
        --container-dir) container_dir=$2; shift 2 ;;
        --up)      run_after=1; shift ;;
        --yes)     assume_yes=1; shift ;;
        --from-source) from_source=1; shift ;;
        --dry-run) dry_run=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) printf 'bernie: unknown flag: %s\n\n' "$1" >&2
           usage >&2
           exit 2 ;;
    esac
done

# `up` starts its own dashboard on the host.
if [ "$run_after" = 1 ] && { [ "$start_after" = 1 ] || [ "$container" = 1 ]; }; then
    printf 'bernie: --up starts the dashboard itself; do not combine it with --start or --container\n' >&2
    exit 2
fi

# bernie ships as a Linux static binary; macOS and Windows are out of scope.
case $(uname -s) in
    Linux) ;;
    *) printf 'bernie: only Linux is supported (got %s)\n' "$(uname -s)" >&2
       exit 1 ;;
esac
case $(uname -m) in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) printf 'bernie: unsupported CPU %s\n' "$(uname -m)" >&2
       exit 1 ;;
esac

if [ "$version" = latest ]; then
    download_base="$base_url/releases/latest/download"
else
    download_base="$base_url/releases/download/$version"
fi
asset="bernie-linux-$arch"

# curl or wget; either works for the install line.
dl() {
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$2" "$1"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$2" "$1"
    else
        printf 'bernie: need curl or wget\n' >&2
        exit 1
    fi
}

# sudo hint, printed only when the chosen dir is not writable.
sudo_hint() {
    printf 'bernie: rerun with sudo:\n' >&2
    printf '    curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sudo sh -s -- --dir %s\n' "$bin_dir" >&2
}

# --- the container path ---------------------------------------------------

# A random hex token for the dashboard. openssl when it is there, else
# /dev/urandom through od.
gen_token() {
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex 24
    else
        od -An -tx1 -N24 /dev/urandom | tr -d ' \n'
    fi
}

# The numeric id of a group on this host. The image has no /etc/group,
# so a group name does not resolve in it: compose needs the number.
group_gid() {
    g=$(getent group "$1" 2>/dev/null | cut -d: -f3)
    if [ -n "$g" ]; then
        printf '%s' "$g"
        return 0
    fi
    # Fall back to the owner of the device node of that group.
    for node in $2; do
        if [ -e "$node" ]; then
            g=$(stat -c %g "$node" 2>/dev/null || stat -f %g "$node" 2>/dev/null || true)
            if [ -n "$g" ]; then
                printf '%s' "$g"
                return 0
            fi
        fi
    done
    printf ''
}

# The compose command of this host: docker compose, podman compose or
# podman-compose. Empty when no engine is there.
pick_compose() {
    if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
        printf 'docker compose'
    elif command -v podman >/dev/null 2>&1 && podman compose version >/dev/null 2>&1; then
        printf 'podman compose'
    elif command -v podman-compose >/dev/null 2>&1; then
        printf 'podman-compose'
    else
        printf ''
    fi
}

# setup_container writes the compose file and the .env of the dashboard
# container, then starts it. The .env holds the dashboard token and the
# GPU group ids of this host; it is mode 0600 because it holds the token.
setup_container() {
    ref=$version
    if [ "$ref" = latest ]; then
        ref=main
    fi
    mkdir -p "$container_dir" || {
        printf 'bernie: cannot create %s\n' "$container_dir" >&2
        return 1
    }
    compose_url="$raw_base/$ref/deploy/compose/compose.yml"
    printf 'bernie: container compose %s\n' "$compose_url"
    if ! dl "$compose_url" "$container_dir/compose.yml"; then
        printf 'bernie: cannot download the compose file\n' >&2
        return 1
    fi
    video_gid=$(group_gid video "/dev/dri/card0 /dev/dri/renderD128")
    render_gid=$(group_gid render "/dev/kfd /dev/dri/renderD128")
    if [ -z "$video_gid" ] || [ -z "$render_gid" ]; then
        printf 'bernie: cannot read the GPU group ids of this host\n' >&2
        printf 'bernie: set VIDEO_GID and RENDER_GID in %s/.env, then start it by hand\n' "$container_dir" >&2
        container_ready=0
    else
        container_ready=1
    fi
    token=$(gen_token)
    cat > "$container_dir/.env" <<EOF
# Written by scripts/install.sh --container. Mode 0600: it holds the token.
BERNIE_DASH_TOKEN=$token
BERNIE_TAG=$version
BERNIE_BIND=127.0.0.1
VIDEO_GID=$video_gid
RENDER_GID=$render_gid
EOF
    chmod 0600 "$container_dir/.env"
    printf 'bernie: container env   %s/.env (mode 0600)\n' "$container_dir"
    printf 'bernie: container bind  127.0.0.1:7860\n'
    printf 'bernie: open            http://127.0.0.1:7860/?token=%s\n' "$token"
    printf 'bernie: the container mounts the GPU device nodes and /sys (read-only); it does not mount the engine socket\n'
}

# --- the source path --------------------------------------------------------

# go_at_least HAVE NEED: true when the dotted version HAVE is at least NEED.
go_at_least() {
    awk -v a="$1" -v b="$2" 'BEGIN {
        n = split(a, x, "."); m = split(b, y, ".")
        for (i = 1; i <= (n > m ? n : m); i++) {
            if (x[i] + 0 > y[i] + 0) exit 0
            if (x[i] + 0 < y[i] + 0) exit 1
        }
        exit 0
    }'
}

# ensure_go GOMOD sets go_bin to a Go that can build the module: the one on
# the PATH when it is new enough, else the toolchain of go.mod in a folder of
# this user. The download is checked against the checksum go.dev publishes.
ensure_go() {
    need=$(sed -n 's/^go \([0-9.]*\).*/\1/p' "$1" | head -n 1)
    if [ -z "$need" ]; then
        printf 'bernie: go.mod names no Go version\n' >&2
        exit 1
    fi
    if command -v go >/dev/null 2>&1; then
        have=$(go env GOVERSION 2>/dev/null | sed 's/^go//')
        if [ -n "$have" ] && go_at_least "$have" "$need"; then
            go_bin=$(command -v go)
            printf 'bernie: go        using %s (go%s)\n' "$go_bin" "$have"
            return 0
        fi
    fi
    want=$(sed -n 's/^toolchain \(go[0-9.]*\).*/\1/p' "$1" | head -n 1)
    if [ -z "$want" ]; then
        want=go$need
        case $need in
            *.*.*) ;;
            *) want=$want.0 ;;
        esac
    fi
    go_bin=$go_dir/$want/bin/go
    if [ -x "$go_bin" ] && "$go_bin" version >/dev/null 2>&1; then
        printf 'bernie: go        using %s\n' "$go_bin"
        return 0
    fi
    file=$want.linux-$arch.tar.gz
    printf 'bernie: go        downloading %s/%s to %s\n' "$go_url" "$file" "$go_dir"
    if ! dl "$go_url/$file" "$work/$file" || ! dl "$go_url/$file.sha256" "$work/$file.sha256"; then
        printf 'bernie: cannot download Go from %s\n' "$go_url" >&2
        exit 1
    fi
    if [ "$(sha256sum "$work/$file" | cut -d' ' -f1)" != "$(tr -d ' \n' < "$work/$file.sha256")" ]; then
        printf 'bernie: the checksum of %s does not match, aborting\n' "$file" >&2
        exit 1
    fi
    mkdir -p "$go_dir/$want.tmp"
    tar -xzf "$work/$file" -C "$go_dir/$want.tmp" --strip-components=1
    rm -rf "${go_dir:?}/$want"
    mv "$go_dir/$want.tmp" "$go_dir/$want"
}

# build_from_source builds the binary into $work/$asset. It needs no git and
# no make: the source is the archive GitHub serves. The module cache and the
# build cache stay next to Go, so a second run is quick.
build_from_source() {
    if [ "$version" = latest ]; then
        label=main
        src_url=$base_url/archive/refs/heads/main.tar.gz
    else
        label=$version
        src_url=$base_url/archive/refs/tags/$version.tar.gz
    fi
    printf 'bernie: source    %s\n' "$src_url"
    if ! dl "$src_url" "$work/src.tar.gz"; then
        printf 'bernie: cannot download the source from %s\n' "$src_url" >&2
        exit 1
    fi
    mkdir -p "$work/src" "$go_dir"
    tar -xzf "$work/src.tar.gz" -C "$work/src" --strip-components=1
    ensure_go "$work/src/go.mod"
    module=$(sed -n 's/^module //p' "$work/src/go.mod")
    stamp=src-$label-$(date -u +%Y%m%d%H%M)
    printf 'bernie: build     %s (a minute or two the first time)\n' "$stamp"
    if ! (cd "$work/src" && CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-modcacherw \
        GOPATH=$go_dir/gopath GOCACHE=$go_dir/cache \
        "$go_bin" build -trimpath -ldflags "-s -w -X $module/internal/cli.Version=$stamp" \
        -o "$work/$asset" ./cmd/bernie </dev/null); then
        printf 'bernie: the build failed\n' >&2
        exit 1
    fi
}

# run_up runs the whole path after the install. A script piped into sh has no
# keyboard on stdin, so the one question of `up` goes to the terminal.
run_up() {
    if [ "$assume_yes" = 1 ]; then
        "$bin_dir/bernie" up --yes </dev/null
    elif [ -t 0 ]; then
        "$bin_dir/bernie" up
    elif (exec </dev/tty) 2>/dev/null; then
        "$bin_dir/bernie" up </dev/tty
    else
        printf 'bernie: no terminal to ask on; rerun with --up --yes, or run: bernie up\n' >&2
        exit 1
    fi
}

# Announce the plan. Always printed so the operator sees what will happen.
printf 'bernie: target    %s @ %s (%s)\n' "$repo" "$version" "$arch"
printf 'bernie: dest      %s/bernie\n' "$bin_dir"
if [ "$from_source" = 1 ]; then
    printf 'bernie: download  none: build from source (Go in %s when needed)\n' "$go_dir"
else
    printf 'bernie: download  %s/%s\n' "$download_base" "$asset"
fi
if [ "$run_after" = 1 ]; then
    printf 'bernie: then      bernie up\n'
fi
if [ "$start_after" = 1 ]; then
    printf 'bernie: dash      start (listen %s)\n' "$listen"
fi
if [ "$container" = 1 ]; then
    printf 'bernie: container %s\n' "$container_dir"
fi

# Dry-run: stop here. The operator has the plan; nothing was touched.
if [ "$dry_run" = 1 ]; then
    printf 'bernie: dry-run: stopping before download\n'
    if [ "$start_after" = 1 ]; then
        printf 'bernie: dry-run: would start the dashboard on %s\n' "$listen"
    fi
    if [ "$container" = 1 ]; then
        printf 'bernie: dry-run: would write the compose file and the .env in %s\n' "$container_dir"
        printf 'bernie: dry-run: would bring the dashboard container up on 127.0.0.1:7860\n'
    fi
    exit 0
fi

# Writability check, BEFORE the download. The script never calls sudo; it
# just reports. Three cases:
#   a) the path exists but is not a directory -> fail
#   b) the path does not exist and we cannot create it -> fail with hint
#   c) the path exists but is not writable -> fail with hint
if [ -e "$bin_dir" ] && [ ! -d "$bin_dir" ]; then
    printf 'bernie: %s exists and is not a directory\n' "$bin_dir" >&2
    exit 1
fi
if [ ! -d "$bin_dir" ]; then
    if mkdir -p "$bin_dir" 2>/dev/null; then :; else
        printf 'bernie: cannot create %s (not writable as %s)\n' "$bin_dir" "$(id -un)" >&2
        sudo_hint
        exit 1
    fi
fi
if [ ! -w "$bin_dir" ]; then
    printf 'bernie: %s is not writable as %s)\n' "$bin_dir" "$(id -un)" >&2
    sudo_hint
    exit 1
fi

# Capture the running version before the upgrade; the dashboard restart is
# skipped when nothing changed.
prev_version=""
if [ -x "$bin_dir/bernie" ]; then
    prev_version=$("$bin_dir/bernie" version 2>/dev/null || printf '')
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/bernie-install.XXXXXX")
trap 'rm -rf "$work"' EXIT INT TERM

if [ "$from_source" = 1 ]; then
    build_from_source
elif dl "$download_base/$asset" "$work/$asset" 2>/dev/null &&
    dl "$download_base/SHA256SUMS" "$work/SHA256SUMS" 2>/dev/null; then
    # Verify the checksum. The expected line is "<sha>  <filename>". A mismatch
    # stops here: it never falls back to the source.
    if ! grep -q " $asset\$" "$work/SHA256SUMS"; then
        printf 'bernie: %s not listed in SHA256SUMS\n' "$asset" >&2
        exit 1
    fi
    (cd "$work" && grep " $asset\$" SHA256SUMS | sha256sum -c -) >/dev/null || {
        printf 'bernie: checksum mismatch, aborting\n' >&2
        exit 1
    }
elif [ "$version" = latest ]; then
    printf 'bernie: no release to download; building from source (branch main)\n'
    build_from_source
else
    printf 'bernie: cannot download release %s from %s\n' "$version" "$download_base" >&2
    printf 'bernie: build that tag instead with --from-source\n' >&2
    exit 1
fi

# Install. Use cp+chmod (POSIX) instead of GNU install(1) for portability.
cp "$work/$asset" "$bin_dir/bernie"
chmod 0755 "$bin_dir/bernie"

# Confirm the binary runs. `bernie version` prints one line.
if ! "$bin_dir/bernie" version >/dev/null 2>&1; then
    printf 'bernie: installed binary does not run on this host\n' >&2
    exit 1
fi

new_version=$("$bin_dir/bernie" version)
printf 'bernie: installed %s\n' "$new_version"

case :$PATH: in
    *:"$bin_dir":*) ;;
    *) printf 'bernie: add %s to your PATH\n' "$bin_dir" ;;
esac

# The container path: the compose file, the .env with the token and the
# GPU group ids, then the container up.
if [ "$container" = 1 ]; then
    if ! setup_container; then
        printf 'bernie: the container is not up; the binary is installed\n' >&2
    elif [ "$container_ready" != 1 ]; then
        printf 'bernie: the container was not started (no GPU group ids on this host)\n' >&2
    else
        compose=$(pick_compose)
        if [ -z "$compose" ]; then
            printf 'bernie: no compose engine found (docker compose, podman compose, podman-compose)\n' >&2
            printf 'bernie: start it by hand: cd %s && docker compose up -d\n' "$container_dir" >&2
        else
            printf 'bernie: starting the container with %s\n' "$compose"
            if (cd "$container_dir" && $compose up -d); then
                printf 'bernie: container up\n'
            else
                printf 'bernie: the container did not start; try: cd %s && %s up -d\n' "$container_dir" "$compose" >&2
            fi
        fi
    fi
fi

# The whole path, when asked.
if [ "$run_after" = 1 ]; then
    run_up
    exit 0
fi

# Start the dashboard when asked. Re-running the line on the same version
# does not restart the dashboard; a version bump does.
if [ "$start_after" = 1 ]; then
    changed=0
    if [ "$prev_version" != "$new_version" ]; then
        changed=1
    fi
    if [ "$changed" = 1 ] || [ "$prev_version" = "" ]; then
        # A loopback listen is the safe default; warn for any other.
        case "$listen" in
            127.0.0.1:*|[::1]*|localhost:*)
                ;;
            *)
                printf 'bernie: WARNING: %s is a non-loopback address; anyone with the URL can drive this host\n' "$listen"
                printf 'bernie: the dashboard will generate a token and require it on every request\n'
                ;;
        esac
        printf 'bernie: starting dashboard on %s ...\n' "$listen"
        if ! "$bin_dir/bernie" dash start --listen "$listen" >/dev/null 2>&1; then
            rc=$?
            printf 'bernie: dash start exited %d; the binary is installed, run `bernie dash start` to retry\n' "$rc"
        fi
    else
        printf 'bernie: version unchanged; dashboard left as-is (run `bernie dash status` to verify)\n'
    fi
    printf 'bernie: dashboard status:\n'
    "$bin_dir/bernie" dash status || true
    printf 'bernie: next step: open the URL above in a browser (Ctrl-C to keep the dashboard running)\n'
else
    printf 'bernie: next step: bernie up\n'
fi
