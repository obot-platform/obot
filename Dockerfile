# First Stage: Builder
FROM cgr.dev/chainguard/wolfi-base AS builder

# Install build dependencies
RUN apk add --no-cache go npm make git pnpm curl

# Set the working directory
WORKDIR /app

# Copy the source code
COPY . .

RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    make in-docker-build

# Second Stage: Final
FROM cgr.dev/chainguard/wolfi-base AS final

# Install build dependencies
RUN apk add --no-cache git python-3.13 py3.13-pip openssh-server npm bash tini
COPY --chmod=0755 /tools/package-chrome.sh /
RUN /package-chrome.sh && rm /package-chrome.sh
RUN sed -E 's/^#(PermitRootLogin)no/\1yes/' /etc/ssh/sshd_config -i
RUN ssh-keygen -A
RUN mkdir /run/sshd && /usr/sbin/sshd


# Copy the compiled application from the builder stage
COPY --from=builder /app/bin/otto8 /bin/
COPY --link --from=builder /app/otto8-tools /otto8-tools
RUN <<EOF
for i in $(find /otto8-tools/[^k]* -name requirements.txt -exec cat {} \; -exec echo \; | sort -u); do
    pip install "$i"
done
EOF
COPY --chmod=0755 <<EOF /bin/run.sh
#!/bin/bash
set -e
mkdir -p /run/sshd
/usr/sbin/sshd -D &
mkdir -p /data/cache
export OTTO_SERVER_VERSIONS="$(cat <<VERSIONS
Tools: $(cd /otto8-tools && git rev-parse HEAD)
WorkspaceProvider: $(cd /otto8-tools/workspace-provider && git rev-parse HEAD)
VERSIONS
)"
exec tini -- otto8 server
EOF

EXPOSE 22
ENV HOME=/data
ENV XDG_CACHE_HOME=/data/cache
ENV GPTSCRIPT_SYSTEM_TOOLS_DIR=/otto8-tools/
ENV OTTO_SERVER_WORKSPACE_TOOL=/otto8-tools/workspace-provider
ENV OTTO_SERVER_TOOL_REGISTRY=/otto8-tools
WORKDIR /data
VOLUME /data
CMD ["run.sh"]
