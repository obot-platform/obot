ARG PROVIDERS_IMAGE=ghcr.io/obot-platform/providers:latest
ARG ENTERPRISE_PROVIDERS_IMAGE=ghcr.io/obot-platform/enterprise-providers:latest
ARG ENCRYPTION_BINS_IMAGE=ghcr.io/obot-platform/providers/encryption-bins:latest
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
  --mount=type=cache,target=/root/go/pkg/mod \
  make all

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

FROM ${PROVIDERS_IMAGE} AS providers
FROM ${ENTERPRISE_PROVIDERS_IMAGE} AS enterprise-providers
FROM ${ENCRYPTION_BINS_IMAGE} AS encryption-bins

FROM final-base AS final
RUN apk add --no-cache bash tini procps curl kubectl jq

COPY aws-encryption.yaml /
COPY azure-encryption.yaml /
COPY gcp-encryption.yaml /
COPY --chmod=0755 run.sh /bin/run.sh

COPY --link --from=providers /obot-providers /obot-providers
COPY --link --from=enterprise-providers /obot-providers /obot-providers
COPY --link --from=encryption-bins /obot-providers /obot-providers
COPY --chmod=0755 /tools/combine-envrc.sh /
RUN /combine-envrc.sh && rm /combine-envrc.sh
COPY --from=encryption-bins /bin/*-encryption-provider /bin/
COPY --from=bin /app/bin/obot /bin/

ENV OBOT_SERVER_DEFAULT_MCPCATALOG_PATH=https://github.com/obot-platform/mcp-catalog
ENV OBOT_SERVER_DEFAULT_SYSTEM_MCPCATALOG_PATH=https://github.com/obot-platform/system-mcp-catalog
ENV OBOT_CONTAINER_ENV=true

ENV POSTGRES_USER=obot
ENV POSTGRES_PASSWORD=obot
ENV POSTGRES_DB=obot
ENV PGDATA=/data/postgresql

ENV HOME=/data
ENV XDG_CACHE_HOME=/data/cache
ENV TERM=vt100

WORKDIR /data
VOLUME /data
ENTRYPOINT ["run.sh"]
