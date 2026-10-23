# Docker development stack

One container with every tool [GETTING_STARTED.md](../docs/GETTING_STARTED.md)
lists (Go, Node.js 24 with pnpm through Corepack, Python 3.12 with the
asset-build modules, ffmpeg, Arial, Nomad 2.0.7), plus the Google Chrome
the asset build renders portraits in. It runs the same sequence as the
native setup: build the game data, start the pinned Nomad dev agent,
provision, `sro-nomad deploy -build`, `pnpm dev`.

This is a development convenience, not a deployment. Nomad is still the
process owner inside the container. The production contracts in
[DEPLOYMENT.md](../apps/server/ops/docs/DEPLOYMENT.md) are unchanged; they
still say containers are not a supported runtime.

## Requirements

Running the stack and building the game data (`assets`) need different
things. Many machines can run the stack but not build the game data; build
it once on a machine that can, and the stack uses the result.

### To run the stack

| | |
|---|---|
| Host | A Linux machine, amd64 or arm64. WSL 2 with Docker Engine inside the distribution is untested; see [Not supported](#not-supported). |
| Kernel | Any kernel current Docker Engine runs on. |
| Docker | Docker Engine 23.0 or newer, the system (rootful) daemon, and the Docker Compose v2 plugin. Tested with Engine 29.8.1 and Compose v5.5.1. |
| Your account | Able to run `docker` (in the `docker` group, or through `sudo`). Run `configure.sh` as yourself, not through `sudo`: it records your uid and gid. |
| git | 2.31 or newer (`configure.sh` uses `rev-parse --path-format`). |
| Ports | Free on 127.0.0.1: Nomad 4646-4648, Agent 8787, 8789 and 8790, GameWorld 8788 (TCP and UDP) and 8791, the client 5180. |
| Disk | About 3 GB for the image, plus the Go and pnpm caches and the game data in the checkout. |
| Network | During `docker compose build` and the first start: Docker Hub, ghcr.io, the Debian mirrors, releases.hashicorp.com, the npm registry and the Go module proxy. |
| Browser | A WebGPU browser on the host (current Chrome or Edge). |
| Game data | Already built in the checkout's `.generated` trees, by `assets` below or by a native build. |

### To build the game data (`assets`)

Everything above, and:

| | |
|---|---|
| Host | amd64 only: the build renders portraits in Google Chrome, which Google publishes for amd64 only. |
| Kernel | Linux 6.13 or newer, with case-insensitive tmpfs. Check with `ls /sys/fs/tmpfs/features/casefold`. For example, Debian 13's stock 6.12 kernel is too old. On an older kernel, see [Older kernels](#older-kernels). |
| GPU | An Intel or AMD GPU on the `i915`, `xe` or `amdgpu` kernel driver (`/dev/dri/renderD*`). NVIDIA GPUs are not supported yet, and Chrome's software renderer does not draw the portraits. |
| Memory | The extracted client lives in RAM while `assets` runs, up to 8 GB (`SRO_DOCKER_EXTRACTED_SIZE`), plus what Chrome and the build use. |
| Network | Also dl.google.com (Chrome) and SourceForge (the Arial fonts, which Debian's `ttf-mscorefonts-installer` downloads during the build). |
| Client | Your licensed Silkroad Online v1.150 client: the Windows client folder with `SRO_Client.exe` and the `.pk2` archives. Nothing runs the `.exe`; the build reads the archives. |

### Not supported

- **Docker Desktop**, on any system. It runs containers in a VM, so host
  networking, host pids and `/dev/dri` name the VM rather than your
  machine. `configure.sh` warns.
- **Rootless Docker** and **Podman**. Their host network is a namespace of
  their own, so the client on 127.0.0.1:5180 is unreachable from your
  browser, and files the container writes are not owned by you.
  `configure.sh` refuses a rootless daemon.
- **macOS and Windows hosts.** Use the native setup in GETTING_STARTED.md
  there. WSL 2 with Docker Engine inside the distribution is Linux to the
  stack but untested: a browser on Windows reaches it only through WSL's
  localhost forwarding, and the client's WebTransport connection to the
  GameWorld is UDP (8788), which WSL's default NAT networking may not
  forward.

## Use

From the checkout (each worktree is configured on its own):

```sh
docker/configure.sh                   # writes .env: your uid/gid, the paths, what the host supports
docker compose build                  # the image, a few minutes once
docker compose run --rm sro assets    # game data from your client, about 6 minutes
docker compose up                     # Nomad, Agent, GameWorld, then the client
```

Open <http://127.0.0.1:5180/> and log in as `tester` / `123123`.

Run `configure.sh` again after pulling changes to the Docker files, after a
kernel or GPU change, or to point at another client folder. It keeps other
lines in `.env`.

**The first asset build on a machine fails** at the `hunting-portraits`
family with `asset-load`. The portraits are drawn by the client viewer,
which reads the asset packs, and a full build writes the packs only after
the families. Until that is fixed upstream, the first build packs what it
has and builds again, in one run:

```sh
docker compose run --rm sro bash -c 'sro-docker assets || { pnpm assets repack && sro-docker assets; }'
```

Later builds use plain `docker compose run --rm sro assets`.

Press Ctrl+C or run `docker compose down` to stop. The container runs
`sro-nomad stop` and waits for the fleet to drain before Nomad exits, the
order GETTING_STARTED.md requires.

Other commands run in the same image:

```sh
docker compose exec sro apps/server/temp/docker/sro-nomad status   # stack running
docker compose run --rm sro pnpm check source                      # the CI gate
docker compose run --rm sro bash                                   # a shell
```

`docker compose run --rm sro setup` only installs packages.
`SRO_BUILD_JOBS` is passed through to the asset build when set.

## What configure.sh decides

`configure.sh` writes `.env`, which Compose reads. It prints what it wrote
and warns about anything the host lacks.

| Setting | Default | Change it with |
|---|---|---|
| The client folder | `SRO_GAME_ROOT` when set; else the folder `.env` already names; else beside the main checkout, where a native run looks | `SRO_GAME_ROOT=/path/to/client docker/configure.sh` |
| The image target | `assets` on amd64, `stack` elsewhere (see [docker/Dockerfile](Dockerfile)) | `SRO_DOCKER_TARGET=stack docker/configure.sh` |
| The case-insensitive `extracted/` ([compose.casefold.yaml](compose.casefold.yaml)) | Added when the kernel supports it | A newer kernel |
| The GPU ([compose.gpu.yaml](compose.gpu.yaml)) | Added with an Intel or AMD GPU | Other hardware |
| The project name | `opensro`; `opensro-<folder>` in a linked worktree | |
| The size of the `extracted/` tmpfs | `8g` | `SRO_DOCKER_EXTRACTED_SIZE=12g` in `.env`; then `docker volume rm <project>_extracted` while no container uses it |

## How it fits the repository

- **Host networking.** Nomad, the Agent (8787, 8789, 8790), the GameWorld
  (8788 TCP+UDP, control 8791) and Vite (5180) all bind 127.0.0.1, and the
  deployer refuses a non-loopback network in the development namespace.
  The container therefore uses the host's loopback, and those ports must
  be free. Do not run a native stack, or a second worktree's stack, at the
  same time. (`SRO_DEV_PORT_OFFSET`, from the
  [Nomad README](../apps/server/ops/nomad/README.md#a-second-local-stack),
  is not wired into the container yet.)
- **Same paths as the host.** The checkout, the main checkout and the
  client folder are mounted where they are on the host. A worktree's `.git`
  file names the main checkout by absolute path, and the resolvers follow
  it to the shared `.generated` trees. The Nomad dev agent identifies a
  checkout by its config and data paths. A native run and a container run
  therefore share `.state`, `.generated` and `node_modules`.
- **Host PIDs.** A store's `authority.lock` keeps its last writer's pid, and
  on Linux a live process with that pid counts as the owner. In a private
  PID namespace, pids restart at 1 on every start, so an old pid would soon
  name a new process and lock the world out. `pid: host` keeps the native
  behaviour. It also means processes in the container can see your other
  processes.
- **Your user.** The container runs as your uid/gid (from `.env`), so the
  files it writes stay editable on the host.
- **SELinux.** On SELinux hosts (Fedora, RHEL), the container would need its
  bind mounts relabelled before it could read them. It turns off SELinux
  separation instead (`label=disable`), since it already shares the host's
  network and pids. Hosts without SELinux ignore the setting.
- **Caches.** Go modules, the Go build cache and the pnpm store live in the
  project's `cache` volume (`opensro_cache` for the main checkout). pnpm's
  store is not the host's, so a host-side `pnpm install` may ask to rebuild
  `node_modules` after a container one.

## The asset build on Linux

The pipeline was written for Windows and macOS. Three things make it run
here:

- **Case-insensitive extraction.** The build opens `regioninfo.txt` for the
  client's `RegionInfo.txt` and `c_rudolph.bsk` for `c_rudolph.BSK`, which
  only a case-insensitive filesystem resolves. On Linux 6.13 or newer,
  `configure.sh` adds [compose.casefold.yaml](compose.casefold.yaml), which
  mounts the client's `extracted/` folder on a casefold tmpfs, and `assets`
  flags it case-insensitive before `prepare` fills it. It is RAM-backed and
  emptied when the container stops, so every `assets` run extracts the
  client again. Nothing else reads it.
- **Chrome and a GPU.** The `hunting-portraits` family renders 386 monster
  portraits through the client's WebGPU viewer in headless Chrome. On
  Chrome's SwiftShader software adapter the viewer never fills its portrait
  target (`No portrait target __doll`). With an Intel or AMD GPU,
  `configure.sh` adds [compose.gpu.yaml](compose.gpu.yaml), which passes
  `/dev/dri` in and points Chrome at Vulkan through the image's Mesa
  drivers.
- **Portable generators.** As on macOS, lens and character mips and the
  font atlas come from portable generators, equivalent to but not
  byte-identical with Windows. The optional ASTC texture variants are
  skipped because the pinned encoder is Windows-only. `pnpm assets doctor`
  reports the platform as a warning.

### Older kernels

Without casefold tmpfs, `extracted/` stays the host folder, and `assets`
stops before it starts unless that folder is case-insensitive. It can be,
on an ext4 or f2fs filesystem created with the casefold feature: format
one (`mkfs.ext4 -O casefold`), mount it at `<client folder>/extracted`
while that folder is empty, and make it yours (`chown`). `assets` then
flags it case-insensitive itself. Unlike the tmpfs, it keeps its contents
between runs and uses disk rather than RAM.

Otherwise, build the game data on another machine, natively or with this
stack, and copy the main checkout's `.generated/` folder across
(`client-public/assets/` and `game-data/1.150/`, as GETTING_STARTED.md
section 2 describes).

## Resetting the world

Stop the stack, then move the state aside as in
[GETTING_STARTED.md section 6](../docs/GETTING_STARTED.md#6-resetting-the-local-world)
(`apps/server/.state/cluster`, `shards`, `nomad/dev-agent`), and start
again.
