ARG TOOLS_IMAGE=ghcr.io/obot-platform/tools:latest
ARG PROVIDER_IMAGE=ghcr.io/obot-platform/tools/providers:latest
ARG ENTERPRISE_IMAGE=cgr.dev/chainguard/wolfi-base:latest

FROM cgr.dev/chainguard/wolfi-base AS base

RUN apk add --no-cache go make git nodejs npm pnpm

FROM base AS bin
WORKDIR /app
COPY . .
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/.cache/uv \
    --mount=type=cache,target=/root/go/pkg/mod \
    make all

FROM cgr.dev/chainguard/postgres:latest-dev AS build-pgvector
RUN apk add build-base git postgresql-dev clang
RUN git clone --branch v0.8.0 https://github.com/pgvector/pgvector.git && \
    cd pgvector && \
    make clean && \
    make OPTFLAGS="" && \
    make install && \
    cd .. && \
    rm -rf pgvector

FROM ${TOOLS_IMAGE} AS tools
FROM ${PROVIDER_IMAGE} AS provider
FROM ${ENTERPRISE_IMAGE} AS enterprise-tools
RUN mkdir -p /obot-tools

FROM cgr.dev/chainguard/postgres:latest-dev AS final
ENV POSTGRES_USER=obot
ENV POSTGRES_PASSWORD=obot
ENV POSTGRES_DB=obot
ENV PGDATA=/data/postgresql

COPY --from=build-pgvector /usr/lib/postgresql17/vector.so /usr/lib/postgresql17/
COPY --from=build-pgvector /usr/share/postgresql17/extension/vector* /usr/share/postgresql17/extension/

RUN apk add --no-cache git python-3.13 py3.13-pip npm nodejs bash tini procps libreoffice docker perl-utils sqlite sqlite-dev curl kubectl jq
COPY --chmod=0755 /tools/package-chrome.sh /
COPY --chmod=0755 /tools/package-mcp-catalog.sh /

RUN /package-chrome.sh && rm /package-chrome.sh
RUN /package-mcp-catalog.sh && rm /package-mcp-catalog.sh && mv catalog.json /catalog.json
ENV OBOT_SERVER_MCPCATALOGS=/catalog.json
COPY aws-encryption.yaml /
COPY azure-encryption.yaml /
COPY gcp-encryption.yaml /
COPY --chmod=0755 run.sh /bin/run.sh

COPY --link --from=tools /obot-tools /obot-tools
COPY --link --from=enterprise-tools /obot-tools /obot-tools
COPY --link --from=provider /obot-tools /obot-tools
COPY --chmod=0755 /tools/combine-envrc.sh /
RUN /combine-envrc.sh && rm /combine-envrc.sh
COPY --from=provider /bin/*-encryption-provider /bin/
COPY --from=bin /app/bin/obot /bin/
COPY --from=bin --link /app/ui/user/build-node /ui

# libreoffice executables
ENV PATH=$PATH:/usr/lib/libreoffice/program
ENV PATH=$PATH:/usr/bin
ENV HOME=/data
ENV XDG_CACHE_HOME=/data/cache
ENV OBOT_SERVER_AGENTS_DIR=/agents
ENV TERM=vt100
WORKDIR /data
VOLUME /data
ENTRYPOINT ["run.sh"]
