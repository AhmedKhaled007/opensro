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

- Linux 6.13 or newer on amd64, with Docker Engine and the Compose plugin
  (v2). The stack uses host networking, so on Docker Desktop host
  networking must be turned on.
- An Intel or AMD GPU (`/dev/dri`) for the asset build. Running the stack
  does not need one.
- Your licensed Silkroad Online v1.150 client: the Windows client folder
  with `SRO_Client.exe` and the `.pk2` archives. Nothing runs the `.exe`;
  the build reads the archives.
- A WebGPU browser on the host (current Chrome or Edge).

## Use

From the checkout (each worktree is configured on its own):

```sh
docker/configure.sh                   # writes .env: your uid/gid, the paths, the GPU
docker compose build                  # the image, a few minutes once
docker compose run --rm sro assets    # game data from your client, about 6 minutes
docker compose up                     # Nomad, Agent, GameWorld, then the client
```

Open <http://127.0.0.1:5180/> and log in as `tester` / `123123`.

`configure.sh` looks for the client where a native run does: beside the
main checkout. Point it elsewhere with
`SRO_GAME_ROOT=/path/to/client docker/configure.sh`.

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

## How it fits the repository

- **Host networking.** Nomad, the Agent (8787, 8789, 8790), the GameWorld
  (8788 TCP+UDP, control 8791) and Vite (5180) all bind 127.0.0.1, and the
  deployer refuses a non-loopback network in the development namespace.
  The container therefore uses the host's loopback, and those ports must
  be free. Do not run a native stack at the same time.
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
- **Caches.** Go modules, the Go build cache and the pnpm store live in the
  `opensro_cache` volume. pnpm's store is not the host's, so a host-side
  `pnpm install` may ask to rebuild `node_modules` after a container one.

## The asset build on Linux

The pipeline was written for Windows and macOS. Three things make it run
here:

- **Case-insensitive extraction.** The build opens `regioninfo.txt` for the
  client's `RegionInfo.txt` and `c_rudolph.bsk` for `c_rudolph.BSK`, which
  only a case-insensitive filesystem resolves. Compose mounts the client's
  `extracted/` folder on a casefold tmpfs (hence Linux 6.13), and `assets`
  flags it case-insensitive before `prepare` fills it. It is RAM-backed and
  emptied when the container stops, so every `assets` run extracts the
  client again. Nothing else reads it.
- **Chrome and a GPU.** The `hunting-portraits` family renders 386 monster
  portraits through the client's WebGPU viewer in headless Chrome. On
  Chrome's SwiftShader software adapter the viewer never fills its portrait
  target (`No portrait target __doll`). With a host GPU, `configure.sh`
  adds `docker/compose.gpu.yaml`, which passes `/dev/dri` in and points
  Chrome at Vulkan.
- **Portable generators.** As on macOS, lens and character mips and the
  font atlas come from portable generators, equivalent to but not
  byte-identical with Windows. The optional ASTC texture variants are
  skipped because the pinned encoder is Windows-only. `pnpm assets doctor`
  reports the platform as a warning.

## Resetting the world

Stop the stack, then move the state aside as in
[GETTING_STARTED.md section 6](../docs/GETTING_STARTED.md#6-resetting-the-local-world)
(`apps/server/.state/cluster`, `shards`, `nomad/dev-agent`), and start
again.
