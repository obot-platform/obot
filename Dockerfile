ARG TOOLS_IMAGE=ghcr.io/obot-platform/tools:latest
ARG PROVIDER_IMAGE=ghcr.io/obot-platform/tools/providers:latest
ARG ENTERPRISE_IMAGE=cgr.dev/chainguard/wolfi-base:latest
ARG BASE_IMAGE=cgr.dev/chainguard/wolfi-base

FROM ${BASE_IMAGE} AS base
ARG BASE_IMAGE
RUN if [ "${BASE_IMAGE}" = "cgr.dev/chainguard/wolfi-base" ]; then \
  apk add --no-cache gcc=14.2.0-r13 go make git nodejs npm pnpm; \
  fi

FROM base AS bin
WORKDIR /app

# Copy dependency manifests and local replace dependencies first (rarely change)
COPY go.mod go.sum ./
COPY apiclient/go.mod apiclient/go.sum ./apiclient/
COPY logger/go.mod logger/go.sum ./logger/
COPY ui/user/package.json ui/user/pnpm-lock.yaml ./ui/user/

# Download main module dependencies (cached unless go.mod changes)
RUN --mount=type=cache,target=/root/go/pkg/mod \
  go mod download

# Install UI dependencies (cached unless package files change)
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
  cd ui/user && pnpm install --frozen-lockfile

# Copy source code including auth provider modules (needed for replace directives)
COPY . .

# Build with cached dependencies (faster rebuilds)
# hadolint ignore=DL3003
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
  --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/root/.cache/uv \
  --mount=type=cache,target=/root/go/pkg/mod \
  make all && \
  cd tools/entra-auth-provider && make build && \
  cd ../keycloak-auth-provider && make build

# Intermediate stage to fetch upstream tools images
FROM cgr.dev/chainguard/wolfi-base:latest AS tools-fetch
RUN apk add --no-cache ca-certificates

FROM ${TOOLS_IMAGE} AS tools
FROM ${PROVIDER_IMAGE} AS provider
FROM ${ENTERPRISE_IMAGE} AS enterprise-tools
RUN mkdir -p /obot-tools

# Create unified tools directory by patching upstream tools with custom auth providers
FROM cgr.dev/chainguard/wolfi-base:latest AS tools-patched
RUN apk add --no-cache yq bash

# Copy upstream tools as base
COPY --from=tools /obot-tools /obot-tools

# Copy provider tools (encryption providers, etc.)
COPY --from=provider /obot-tools /obot-tools
COPY --from=enterprise-tools /obot-tools /obot-tools

# Create directories for custom auth providers
RUN mkdir -p /obot-tools/tools/entra-auth-provider/bin && \
  mkdir -p /obot-tools/tools/keycloak-auth-provider/bin && \
  mkdir -p /obot-tools/tools/placeholder-credential && \
  mkdir -p /obot-tools/tools/auth-providers-common/templates

# Copy custom auth provider binaries
COPY --from=bin /app/tools/entra-auth-provider/bin/gptscript-go-tool /obot-tools/tools/entra-auth-provider/bin/
COPY --from=bin /app/tools/entra-auth-provider/tool.gpt /obot-tools/tools/entra-auth-provider/

# Copy keycloak auth provider binaries
COPY --from=bin /app/tools/keycloak-auth-provider/bin/gptscript-go-tool /obot-tools/tools/keycloak-auth-provider/bin/
COPY --from=bin /app/tools/keycloak-auth-provider/tool.gpt /obot-tools/tools/keycloak-auth-provider/

# Copy shared dependencies used by auth providers
COPY --from=bin /app/tools/placeholder-credential/ /obot-tools/tools/placeholder-credential/
COPY --from=bin /app/tools/auth-providers-common/templates/ /obot-tools/tools/auth-providers-common/templates/

# Copy and merge custom tool index with upstream index
COPY --from=bin /app/tools/index.yaml /tmp/custom-index.yaml

# Merge custom authProviders into existing upstream index.yaml
# This combines upstream tools (github, google auth) with custom (entra, keycloak)
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
# hadolint ignore=SC2016
RUN if [ -f /obot-tools/tools/index.yaml ]; then \
  # Extract upstream authProviders (if any exist)
  upstream_auth=$(yq '.authProviders' /obot-tools/tools/index.yaml 2>/dev/null || echo "{}"); \
  custom_auth=$(yq '.authProviders' /tmp/custom-index.yaml 2>/dev/null || echo "{}"); \
  # Merge authProviders sections
  if [ "$upstream_auth" = "null" ] || [ "$upstream_auth" = "{}" ]; then \
  yq eval '.authProviders = '"$(echo "$custom_auth" | yq -I4 -)"'' /obot-tools/tools/index.yaml > /tmp/merged-index.yaml; \
  else \
  # Deep merge: preserve upstream, add custom providers
  yq eval-all 'select(fileIndex == 0) as $upstream | select(fileIndex == 1) as $custom | $upstream * {"authProviders": ($upstream.authProviders + $custom.authProviders)}' \
  /obot-tools/tools/index.yaml /tmp/custom-index.yaml > /tmp/merged-index.yaml; \
  fi; \
  mv /tmp/merged-index.yaml /obot-tools/tools/index.yaml; \
  else \
  # No upstream index, use custom index directly
  cp /tmp/custom-index.yaml /obot-tools/tools/index.yaml; \
  fi && \
  rm /tmp/custom-index.yaml

# Copy tool.gpt wrapper (outputfilter hack for loading index.yaml)
COPY --from=bin /app/tools/tool.gpt /obot-tools/tools/

FROM cgr.dev/chainguard/wolfi-base:latest AS final-base
RUN addgroup -g 70 postgres && \
  adduser -u 70 -G postgres -h /home/postgres -s /bin/sh postgres -D

ENV PGDATA=/var/lib/postgresql/data
ENV LANG=en_US.UTF-8
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/bin:/usr/sbin:/sbin:/bin
WORKDIR /home/postgres

RUN apk add --no-cache postgresql-17 postgresql-17-oci-entrypoint postgresql-17-client postgresql-17-contrib gosu ecpg-17 glibc-locale-en glibc-locale-posix posix-libc-utils

ENTRYPOINT [ "/usr/bin/docker-entrypoint.sh", "postgres" ]

FROM final-base AS build-pgvector
RUN apk add --no-cache build-base git postgresql-17-dev clang-19
# hadolint ignore=DL3003
RUN git clone --branch v0.8.1 https://github.com/pgvector/pgvector.git && \
  cd pgvector && \
  make clean && \
  make OPTFLAGS="" && \
  PG_MAJOR=17 make install && \
  cd .. && \
  rm -rf pgvector

FROM final-base AS final
ENV POSTGRES_USER=obot
ENV POSTGRES_PASSWORD=obot
ENV POSTGRES_DB=obot
ENV PGDATA=/data/postgresql

COPY --from=build-pgvector /usr/lib/postgresql17/vector.so /usr/lib/postgresql17/
COPY --from=build-pgvector /usr/share/postgresql17/extension/vector* /usr/share/postgresql17/extension/

RUN apk add --no-cache git python-3.13 py3.13-pip npm nodejs bash tini procps libreoffice docker perl-utils sqlite sqlite-dev curl kubectl jq

ENV OBOT_SERVER_DEFAULT_MCPCATALOG_PATH=https://github.com/obot-platform/mcp-catalog

COPY aws-encryption.yaml /
COPY azure-encryption.yaml /
COPY gcp-encryption.yaml /
COPY --chmod=0755 run.sh /bin/run.sh

# Copy unified tools directory with all upstream tools and custom auth providers merged
COPY --link --from=tools-patched /obot-tools /obot-tools

# Combine all .envrc files from upstream tools, enterprise tools, and providers
COPY --chmod=0755 /tools/combine-envrc.sh /
RUN /combine-envrc.sh && rm /combine-envrc.sh

COPY --from=provider /bin/*-encryption-provider /bin/
COPY --from=bin /app/bin/obot /bin/
COPY --from=bin --link /app/ui/user/build-node /ui

ENV PATH=$PATH:/usr/lib/libreoffice/program
ENV PATH=$PATH:/usr/bin
ENV HOME=/data
ENV XDG_CACHE_HOME=/data/cache
ENV OBOT_SERVER_AGENTS_DIR=/agents
ENV TERM=vt100
ENV OBOT_CONTAINER_ENV=true
# Unified tool registry containing upstream tools and custom auth providers
# All providers are in /obot-tools/tools directory with merged index.yaml
ENV OBOT_SERVER_TOOL_REGISTRIES=/obot-tools/tools
WORKDIR /data
VOLUME /data
ENTRYPOINT ["run.sh"]
