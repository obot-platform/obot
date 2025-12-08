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
COPY . .
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
  --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/root/.cache/uv \
  --mount=type=cache,target=/root/go/pkg/mod \
  make all && \
  cd tools/entra-auth-provider && make build

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
RUN git clone --branch v0.8.1 https://github.com/pgvector/pgvector.git && \
  cd pgvector && \
  make clean && \
  make OPTFLAGS="" && \
  PG_MAJOR=17 make install && \
  cd .. && \
  rm -rf pgvector

FROM ${TOOLS_IMAGE} AS tools
FROM ${PROVIDER_IMAGE} AS provider
FROM ${ENTERPRISE_IMAGE} AS enterprise-tools
RUN mkdir -p /obot-tools

FROM final-base AS final
ENV POSTGRES_USER=obot
ENV POSTGRES_PASSWORD=obot
ENV POSTGRES_DB=obot
ENV PGDATA=/data/postgresql

COPY --from=build-pgvector /usr/lib/postgresql17/vector.so /usr/lib/postgresql17/
COPY --from=build-pgvector /usr/share/postgresql17/extension/vector* /usr/share/postgresql17/extension/

RUN apk add --no-cache git python-3.13 py3.13-pip npm nodejs bash tini procps libreoffice docker perl-utils sqlite sqlite-dev curl kubectl jq
COPY --chmod=0755 /tools/package-chrome.sh /

RUN /package-chrome.sh && rm /package-chrome.sh
ENV OBOT_SERVER_DEFAULT_MCPCATALOG_PATH=https://github.com/obot-platform/mcp-catalog

COPY aws-encryption.yaml /
COPY azure-encryption.yaml /
COPY gcp-encryption.yaml /
COPY --chmod=0755 run.sh /bin/run.sh

COPY --link --from=tools /obot-tools /obot-tools
COPY --link --from=enterprise-tools /obot-tools /obot-tools
COPY --link --from=provider /obot-tools /obot-tools

# Copy custom Entra ID auth provider to separate directory to avoid conflicts
COPY --from=bin /app/tools/index.yaml /obot-tools-custom/
COPY --from=bin /app/tools/entra-auth-provider/tool.gpt /obot-tools-custom/entra-auth-provider/
COPY --from=bin /app/tools/entra-auth-provider/bin/gptscript-go-tool /obot-tools-custom/entra-auth-provider/
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
# Include custom Entra ID auth provider registry alongside default tools
ENV OBOT_SERVER_TOOL_REGISTRIES=/obot-tools-custom
WORKDIR /data
VOLUME /data
ENTRYPOINT ["run.sh"]
