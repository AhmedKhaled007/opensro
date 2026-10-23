# ===========================================================================
# Dockerfile - the OpenSRO development toolchain and local stack in one image
#
# One Linux image with the tools docs/GETTING_STARTED.md lists: Go, Node.js
# with pnpm through Corepack, Python 3.12 with the asset-build modules,
# ffmpeg, Arial for the font atlas, and the pinned Nomad. The checkout is
# bind-mounted, not copied, so builds and state land in the same .generated
# and .state folders a native run uses.
#
# Nomad stays the process owner inside the container: docker/entrypoint.sh
# runs the documented dev-agent / bootstrap / deploy sequence and never
# starts agent or gameworld itself (apps/server/ops/docs/DEPLOYMENT.md).
# ===========================================================================

# Go follows apps/server/go.mod; Node the 24 LTS line CI runs.
ARG GO_VERSION=1.27.3
ARG NODE_VERSION=24.13.1
ARG UV_VERSION=0.9.5

FROM node:${NODE_VERSION}-trixie AS node
FROM ghcr.io/astral-sh/uv:${UV_VERSION} AS uv

FROM golang:${GO_VERSION}-trixie

# Nomad must be the version sro-nomad dev-agent pins; pnpm the one
# package.json packageManager names.
ARG NOMAD_VERSION=2.0.7
ARG PNPM_VERSION=12.6.0
ARG TARGETARCH
# The container user matches the checkout's owner so files it writes into
# the bind mount stay editable on the host.
ARG UID=1000
ARG GID=1000

# ffmpeg converts the client music, unar opens a client shipped as .rar,
# e2fsprogs' chattr makes the extraction folder case-insensitive
# (entrypoint.sh casefold_extracted), Mesa's Vulkan drivers let Chrome use
# a host Intel or AMD GPU (docker/compose.gpu.yaml), and the Microsoft core
# fonts (Debian contrib) supply the Arial face that
# scripts/build/native_ui_font_atlas.py looks for on Linux.
RUN sed -i 's/^Components: main$/Components: main contrib/' /etc/apt/sources.list.d/debian.sources \
	&& echo 'ttf-mscorefonts-installer msttcorefonts/accepted-mscorefonts-eula select true' | debconf-set-selections \
	&& apt-get update \
	&& apt-get install -y --no-install-recommends e2fsprogs ffmpeg mesa-vulkan-drivers ttf-mscorefonts-installer unar unzip \
	&& rm -rf /var/lib/apt/lists/* \
	&& test -f /usr/share/fonts/truetype/msttcorefonts/Arial.ttf \
	&& test -f /usr/share/fonts/truetype/msttcorefonts/Arial_Bold.ttf

# Google Chrome: the asset build renders monster and catalogue portraits in
# it (scripts/lib/probeBrowser.mjs launches the "chrome" channel), from
# Google's signed repository. Google publishes it for amd64 only.
RUN test "${TARGETARCH}" = amd64 \
	&& curl -fsSL https://dl.google.com/linux/linux_signing_key.pub \
		| gpg --dearmor -o /usr/share/keyrings/google-chrome.gpg \
	&& echo "deb [arch=amd64 signed-by=/usr/share/keyrings/google-chrome.gpg] https://dl.google.com/linux/chrome/deb/ stable main" \
		> /etc/apt/sources.list.d/google-chrome.list \
	&& apt-get update \
	&& apt-get install -y --no-install-recommends google-chrome-stable \
	&& rm -rf /var/lib/apt/lists/* \
	&& google-chrome --version

# Node.js and its bundled npm and Corepack, taken from the official image.
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
ENV COREPACK_HOME=/opt/corepack \
	COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
	&& ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
	&& ln -s ../lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack \
	&& corepack enable --install-directory /usr/local/bin pnpm \
	&& corepack install --global pnpm@${PNPM_VERSION} \
	&& pnpm --version
# The first pnpm run above also fetches its native binary into COREPACK_HOME,
# so containers do not download it again on every start.

# Python 3.12 in a venv, as the macOS setup does with uv. SRO_PYTHON is the
# interpreter every asset build step runs.
COPY --from=uv /uv /usr/local/bin/uv
COPY requirements-build.txt /tmp/requirements-build.txt
ENV UV_PYTHON_INSTALL_DIR=/opt/uv-python
RUN uv venv --python 3.12 /opt/venv \
	&& uv pip install --no-cache --python /opt/venv/bin/python -r /tmp/requirements-build.txt \
	&& rm /tmp/requirements-build.txt
ENV SRO_PYTHON=/opt/venv/bin/python \
	PATH=/opt/venv/bin:$PATH

# The pinned Nomad, checked against HashiCorp's published checksum the way
# GETTING_STARTED.md installs it. dev-agent finds it on PATH.
RUN cd /tmp \
	&& file="nomad_${NOMAD_VERSION}_linux_${TARGETARCH}.zip" \
	&& curl -fsSLO "https://releases.hashicorp.com/nomad/${NOMAD_VERSION}/${file}" \
	&& curl -fsSL "https://releases.hashicorp.com/nomad/${NOMAD_VERSION}/nomad_${NOMAD_VERSION}_SHA256SUMS" \
		| grep " ${file}\$" | sha256sum -c - \
	&& unzip -o "${file}" nomad -d /usr/local/bin \
	&& rm "${file}" \
	&& nomad version

RUN groupadd -o -g ${GID} sro \
	&& useradd -o -m -u ${UID} -g ${GID} -s /bin/bash sro \
	&& mkdir -p /cache \
	&& chown -R sro:sro /cache /opt/corepack

# Go and pnpm caches live in a named volume so rebuilds stay incremental.
ENV GOMODCACHE=/cache/go/mod \
	GOCACHE=/cache/go/build \
	pnpm_config_store_dir=/cache/pnpm-store

COPY --chmod=755 docker/entrypoint.sh /usr/local/bin/sro-docker
USER sro
ENTRYPOINT ["sro-docker"]
CMD ["stack"]
