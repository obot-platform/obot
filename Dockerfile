ARG BASE_IMAGE=cgr.dev/chainguard/wolfi-base

FROM ${BASE_IMAGE} AS base
ARG BASE_IMAGE
RUN if [ "${BASE_IMAGE}" = "cgr.dev/chainguard/wolfi-base" ]; then \
  apk add --no-cache bash gcc=14.2.0-r13 go make git nodejs npm pnpm; \
  fi

FROM base AS bin
WORKDIR /app
COPY . .
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
  --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/root/.cache/uv \
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

FROM base AS provider
WORKDIR /obot-tools
RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/root/go/pkg/mod \
  BIN_DIR=/bin bash -euxo pipefail -c '\
    mkdir -p "${BIN_DIR}"; \
    cd /obot-tools; \
    if [ ! -e aws-encryption-provider ]; then \
      git clone --depth=1 https://github.com/kubernetes-sigs/aws-encryption-provider; \
    fi; \
    cd /obot-tools/aws-encryption-provider; \
    go build -o "${BIN_DIR}/aws-encryption-provider" cmd/server/main.go; \
    cd /obot-tools; \
    if [ ! -e kubernetes-kms ]; then \
      git clone --depth=1 https://github.com/Azure/kubernetes-kms; \
    fi; \
    cd /obot-tools/kubernetes-kms; \
    go build -ldflags="-s -w" -o "${BIN_DIR}/azure-encryption-provider" cmd/server/main.go; \
    cd /obot-tools; \
    if [ ! -e k8s-cloudkms-plugin ]; then \
      git clone --depth=1 https://github.com/obot-platform/k8s-cloudkms-plugin; \
    fi; \
    cd /obot-tools/k8s-cloudkms-plugin; \
    go build -ldflags="-s -w -extldflags static" -installsuffix cgo -tags netgo -o "${BIN_DIR}/gcp-encryption-provider" cmd/k8s-cloudkms-plugin/main.go \
  '

FROM final-base AS build-pgvector
RUN apk add --no-cache build-base git postgresql-17-dev clang-19
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
ENV OBOT_SERVER_DEFAULT_SYSTEM_MCPCATALOG_PATH=https://github.com/obot-platform/system-mcp-catalog

COPY aws-encryption.yaml /
COPY azure-encryption.yaml /
COPY gcp-encryption.yaml /
COPY --chmod=0755 run.sh /bin/run.sh

COPY --from=provider /bin/*-encryption-provider /bin/
COPY --from=bin /app/bin/obot /bin/
COPY --from=bin --link /app/ui/user/build-node /ui

ENV PATH=$PATH:/usr/lib/libreoffice/program
ENV PATH=$PATH:/usr/bin
ENV HOME=/data
ENV XDG_CACHE_HOME=/data/cache
ENV TERM=vt100
ENV OBOT_CONTAINER_ENV=true
ENV OBOT_SERVER_PROVIDER_REGISTRIES=https://github.com/obot-platform/providers
WORKDIR /data
VOLUME /data
ENTRYPOINT ["run.sh"]
