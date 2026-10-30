# Install bernie

bernie is one static Linux binary (amd64 and arm64). Pick one method.
No method needs root, except the package manager and the container engine.
bernie never runs `sudo` by itself.

| Method | Use it when |
|---|---|
| [Installer script](#1-installer-script) | You want the newest binary in `~/.local/bin`. |
| [rpm or deb package](#2-rpm-or-deb-package) | You want the package manager to own the file. |
| [Container image](#3-container-image) | You want only the dashboard, `hw`, `topo`, `doctor`, `bench` in a container. |
| [Compose](#4-compose) | Same, with a file you can keep. |
| [Build from source](#5-build-from-source) | You develop bernie. |

After you install, run `bernie up` (it does the doctor, the setup, the
recipe and the dashboard in one go) or `bernie doctor` for the checks alone.

## 1. Installer script

One command for the whole path: install bernie, then run `bernie up`.

```bash
curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh -s -- --up
```

`up` asks once, on your terminal. Add `--yes` to skip the question. `up` stops
at a step that needs root, such as ROCm or a group, and prints the command. Run
it, then run `bernie up` again.

Without `--up`, the script only installs:

```bash
curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh
```

The script:

1. detects the OS and the CPU (Linux on amd64 or arm64; nothing else);
2. downloads `bernie-linux-<arch>` and `SHA256SUMS` from the latest release;
3. checks the SHA-256 sum and stops if it does not match;
4. installs the file as `~/.local/bin/bernie`;
5. prints the next command: `bernie up`.

### When no release exists

When the latest release cannot be downloaded, the script builds bernie from
the `main` branch instead. It needs no git, no make and no root:

1. it downloads the source archive of the branch;
2. it uses the `go` on your `PATH` when it is new enough for `go.mod`;
3. otherwise it downloads the toolchain that `go.mod` names from go.dev, checks
   its SHA-256 sum, and keeps it in `~/.local/share/bernie/go`. It does not touch
   a Go you already have;
4. it builds and installs the binary.

A build of `main` has no release checksum: you trust GitHub and the branch. A
checksum mismatch of a release never falls back to the source, and neither does
a `--version` tag that has no release. `--from-source` builds on purpose, also
for a tag.

It never calls `sudo` by itself. If the directory you choose is not writable
by your user, the script says so and prints the exact command to run with
`sudo`; you decide whether to run it.

### Flags

| Flag | Meaning |
|---|---|
| `--version TAG` | Release tag, for example `v0.1.0`. Default: the latest release. |
| `--dir DIR` | Install directory. Default: `~/.local/bin`. Use `--dir /usr/local/bin` for a system-wide install (needs `sudo`). |
| `--up` | Run `bernie up` after the install. Does not combine with `--start` or `--container`. |
| `--yes` | With `--up`, do not ask for confirmation. Without it and without a terminal, the script stops and says so. |
| `--from-source` | Build from source even when a release exists. |
| `--dry-run` | Print the plan (target, destination, download URL) and stop. Nothing is downloaded or written. |
| `-h`, `--help` | Print the help. |

### Settings (environment variables)

A flag wins over the variable of the same meaning.

| Variable | Default | Meaning |
|---|---|---|
| `BERNIE_VERSION` | `latest` | Release tag, for example `v0.1.0`. |
| `BERNIE_BIN_DIR` | `~/.local/bin` | Install directory. |
| `BERNIE_REPO` | `opengfx1030/bernie` | GitHub `owner/repo`. |
| `BERNIE_GO_DIR` | `~/.local/share/bernie/go` | Where a build keeps Go and its caches. |

### Examples

```bash
# See what would happen, install nothing.
curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh -s -- --dry-run

# A fixed release into ~/.local/bin.
curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh -s -- --version v0.1.0

# System-wide. The script prints the sudo command; you run it.
curl -fsSL https://raw.githubusercontent.com/opengfx1030/bernie/main/scripts/install.sh | sh -s -- --dir /usr/local/bin
```

### The test

`scripts/test-install.sh` runs the installer against a fake release server
(a local `python3` HTTP server) in a temp dir. It never touches
`~/.local/bin` and never reaches the network.

```bash
make installtest
```

## 2. rpm or deb package

Each release has these files (`<ver>` has no leading `v`):

| File | System |
|---|---|
| `bernie-<ver>-1.x86_64.rpm`, `bernie-<ver>-1.aarch64.rpm` | Fedora, RHEL, openSUSE |
| `bernie_<ver>_amd64.deb`, `bernie_<ver>_arm64.deb` | Debian, Ubuntu |
| `bernie_<ver>_linux_<arch>.tar.gz` | Any Linux: binary, `LICENSE`, `README.md` |
| `SHA256SUMS`, `SHA256SUMS.bundle` | Checksums and their signature |

The package installs `/usr/bin/bernie` and the documents in
`/usr/share/doc/bernie/`. It has no install scripts. It does not change
groups, users or GPU settings.

### Check the download

Check the checksums first:

```bash
sha256sum --ignore-missing -c SHA256SUMS
```

Then check the signature of `SHA256SUMS`. The release workflow signs it with
[cosign](https://docs.sigstore.dev/cosign/) (keyless). A good result proves
that GitHub Actions built the release from this repository:

```bash
cosign verify-blob \
  --bundle SHA256SUMS.bundle \
  --certificate-identity-regexp '^https://github.com/opengfx1030/bernie/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
```

### Install

```bash
sudo dnf install ./bernie-<ver>-1.x86_64.rpm      # Fedora, RHEL
sudo apt install ./bernie_<ver>_amd64.deb         # Debian, Ubuntu
```

## 3. Container image

```bash
docker pull ghcr.io/opengfx1030/bernie:v0.1.0      # or podman pull
```

The image has multi-arch tags: `vX.Y.Z` and `latest`. It holds only the
`bernie` binary on `scratch` (no shell, no libc, no CA bundle) and runs as user 65532 (not root).
The default command is `dash --listen 0.0.0.0:7860`.

### What works in the image

| Works | Does not work |
|---|---|
| `dash`, `hw`, `topo`, `doctor`, `recipe`, `bench` | `serve` in host mode: the image has no Python venv. |
| Reading `/sys` and `/proc` | `serve` in container mode: the image has no `docker` or `podman` program. |

Container-mode `serve` needs an engine program and the engine socket in the
container. To get it, make your own image from this one and add the program.
For all vLLM runs, use the `bernie` binary on the host.

### Run

The dashboard listens on a non-loopback address inside the container.
Therefore it needs a token. Set `BERNIE_DASH_TOKEN`. If you do not, bernie
makes a random token and prints the URL in the log (`docker logs`).

```bash
mkdir -p data && sudo chown 65532:65532 data
docker run -d --name bernie-dash \
  -p 127.0.0.1:7860:7860 \
  -e BERNIE_DASH_TOKEN="$(openssl rand -hex 24)" \
  --device /dev/kfd --device /dev/dri \
  --group-add "$(stat -c %g /dev/kfd)" --group-add "$(stat -c %g /dev/dri/renderD128)" \
  --security-opt label=disable --security-opt seccomp=unconfined \
  -v "$PWD/data:/data" -v /sys:/sys:ro \
  ghcr.io/opengfx1030/bernie:v0.1.0
```

Notes:

- **GPU access.** `/dev/kfd` and `/dev/dri` belong to the groups `render` and
  `video`. The image has no `/etc/group`, so a group name does not resolve
  in it: use the numeric group ids of your host.
  `getent group video` and `getent group render` print them.
  `stat -c %g /dev/kfd /dev/dri/renderD128 /dev/dri/card0` prints the owner
  of every device node.
- **Rootless Podman.** Use `--group-add keep-groups --user 0:0` instead. The
  container root is your own user on the host. Its user namespace does not
  map the group ids of the host, so `keep-groups` is the only way to keep
  the access to the GPU devices.
- **Data.** `/data` holds config, state, data and cache
  (`BERNIE_CONFIG_DIR`, `BERNIE_STATE_DIR`, `BERNIE_DATA_DIR`,
  `BERNIE_CACHE_DIR`).
- **Port.** Publish on `127.0.0.1`. Publish on `0.0.0.0` only on a network you
  trust. The token is then the only protection.
- **Engine socket.** Do not mount it unless you must. Access to the socket
  is the same as root on the host. See [SECURITY.md](../SECURITY.md).
- **Health check.** The image has no shell, so it has no `HEALTHCHECK`. Test
  from outside: `curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:7860/api/hw`.

### Container without the installer

This method runs no script and needs no `sudo`. Docker builds the image from
the repository, so it also works before a release has an image to pull.

```bash
docker build -t bernie -f deploy/docker/Dockerfile https://github.com/opengfx1030/bernie.git
docker run -d --name bernie-dash --restart unless-stopped \
  -p 127.0.0.1:7860:7860 \
  --device /dev/kfd --device /dev/dri \
  --group-add "$(stat -c %g /dev/kfd)" --group-add "$(stat -c %g /dev/dri/renderD128)" \
  --security-opt label=disable --security-opt seccomp=unconfined \
  -v bernie-data:/data -v /sys:/sys:ro \
  bernie
docker logs bernie-dash
```

The log prints the URL to open. A new token is made at each start, unless you
set `BERNIE_DASH_TOKEN`.

The named volume `bernie-data` takes its owner from the image, so it needs no
`chown`. To update, run the build again, run `docker rm -f bernie-dash`, then
run `docker run` again. The volume keeps your data.

The container has the limits of [What works in the image](#what-works-in-the-image).

### GPU access: classic or CDI

A container gets the GPU in one of two ways.

- **Classic.** The device nodes of the host are passed in: `--device /dev/kfd
  --device /dev/dri`. Every GPU of the host is visible in the container.
- **CDI.** The AMD Container Toolkit injects the GPUs through a CDI spec:
  `--device amd.com/gpu=0`. Only the selected GPUs are visible in the
  container.

The toolkit and the spec (both need root):

```bash
sudo dnf install -y amd-container-toolkit    # or: sudo apt install -y amd-container-toolkit
sudo amd-ctk cdi generate --output=/etc/cdi/amd.json
sudo amd-ctk cdi validate
```

Write the spec again when a GPU is added or removed: a stale spec names
nodes that the host does not have. Docker reads CDI from 28.3.0 on, Podman
from 4.0 on.

bernie uses the classic form unless the host has a valid spec and the
engine can read it. The recipe field `container.device_access`, the flag
`--device-access` and the config key `device_access` choose the form:
`classic`, `cdi` or `auto` (the default).

The group flags stay in both forms. The spec sets the owner and the mode of
the injected nodes, but it adds no group to the process of the container.
On a SELinux host the container still needs `--security-opt label=disable`.
Rootless Podman still needs `--group-add keep-groups`, and the user must be
in the `video` and `render` groups.

## 4. Compose

`deploy/compose/compose.yml` is a reference file for the same container.
It works with `docker compose`, `podman compose` and `podman-compose`.

```bash
cd deploy/compose
cp .env.example .env          # set BERNIE_DASH_TOKEN, VIDEO_GID, RENDER_GID
mkdir -p data && sudo chown 65532:65532 data
docker compose up -d          # open http://127.0.0.1:7860
```

For rootless Podman, add the override file:

```bash
podman compose -f compose.yml -f compose.podman.yml up -d
```

The override needs `!override` support (docker compose 2.24 or later,
podman-compose 1.3 or later).

## 5. Build from source

You need Go 1.26 or later.

```bash
git clone https://github.com/opengfx1030/bernie && cd bernie
make build            # bin/bernie
make dist             # dist/: static binaries, tar.gz archives, SHA256SUMS
make packages         # dist/: rpm and deb (runs nfpm through `go run`)
make image            # container image (docker or podman)
```

`make dist` builds both `linux/amd64` and `linux/arm64`. Set `VERSION=v1.2.3`
to override the version taken from `git describe`.

## 6. Download a model

`bernie setup apply --model <repo>` puts the files of a Hugging Face repo
in the cache: `$HF_HOME/hub`, else `~/.cache/huggingface/hub`. A container
run mounts the same cache, so host and container mode share the model.

```bash
bernie setup plan --model Qwen/Qwen3-8B          # the plan, nothing changes
bernie setup apply --model Qwen/Qwen3-8B --yes   # the download
```

The download continues where it stopped. An interrupted download keeps its
partial file, and the next run asks for the rest with a `Range` request.
The checksum of every file is checked before it is moved in place. The
progress is one line every two seconds, and one at the end:

```
model.download: 1.8 GB of 2.3 GB (78%) at 45.2 MB/s
model.download: done, 2.3 GB in 51s (45.4 MB/s)
```

`HF_TOKEN` authenticates for a private repo. `HF_ENDPOINT` points at a
mirror. The `hf` tool of the `huggingface_hub` package does the same job by
hand: `hf download <repo>`.
