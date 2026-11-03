#!/bin/bash
set -e

if [ "${1:-}" = "tunnel" ]; then
  exec tini -- obot "$@"
fi

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

source /obot-providers/.envrc.providers

mkdir -p /data/cache

if [ -z "$OBOT_SERVER_DSN" ]; then
  if [ -f "$PGDATA/PG_VERSION" ]; then
    postgres_version=$(cat "$PGDATA/PG_VERSION")
    if [ "$postgres_version" != "18" ]; then
      echo "The embedded database at $PGDATA was created by PostgreSQL ${postgres_version:-unknown}, but this version of Obot uses PostgreSQL 18." >&2
      echo "See https://docs.obot.ai/next/operations/embedded-postgresql-upgrade to migrate your data." >&2
      exit 1
    fi
  fi

  echo "OBOT_SERVER_DSN is not set. Starting PostgreSQL process..."

  # Start PostgreSQL in the background
  echo "Starting PostgreSQL server..."

  /usr/bin/docker-entrypoint.sh postgres &

  check_postgres_active
  export OBOT_SERVER_DSN="postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:5432/${POSTGRES_DB}"
fi

exec tini -- obot server
