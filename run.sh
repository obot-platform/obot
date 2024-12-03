#!/bin/bash
set -e

check_postgres_active() {
  for i in {1..30}; do
    if pg_isready -q; then
      echo "PostgreSQL is active and ready!"
      return 0
    fi
    echo "Waiting for PostgreSQL to become active... ($i/10)"
    sleep 2
  done
  echo "PostgreSQL did not become active in time."
  exit 1
}

mkdir -p /run/sshd
/usr/sbin/sshd -D &
mkdir -p /data/cache
# This is YAML
export OTTO8_SERVER_VERSIONS="$(cat <<VERSIONS
"github.com/otto8-ai/tools": "$(cd /otto8-tools && git rev-parse HEAD)"
"github.com/gptscript-ai/workspace-provider": "$(cd /otto8-tools/workspace-provider && git rev-parse HEAD)"
"github.com/gptscript-ai/datasets": "$(cd /otto8-tools/datasets && git rev-parse HEAD)"
"github.com/kubernetes-sigs/aws-encryption-provider": "$(cd /otto8-tools/aws-encryption-provider && git rev-parse HEAD)"
# double echo to remove trailing whitespace
"chrome": "$(echo $(/opt/google/chrome/chrome --version))"
VERSIONS
)"

if [ -z "$OTTO8_SERVER_DSN" ]; then
  echo "OTTO8_SERVER_DSN is not set. Starting PostgreSQL process..."

  # Start PostgreSQL in the background
  echo "Starting PostgreSQL server..."
  /usr/bin/docker-entrypoint.sh postgres &

  check_postgres_active
  export OTTO8_SERVER_DSN="postgresql://otto8:otto8@localhost:5432/otto8"
fi

exec tini -- otto8 server
