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

source /obot-tools/.envrc.tools
export PATH=$TOOLS_VENV_BIN:$PATH

# double echo to remove trailing whitespace
export OBOT_SERVER_VERSIONS="$(cat <<VERSIONS
"chrome": "$(echo $(/opt/google/chrome/chrome --version))"
${OBOT_SERVER_VERSIONS}
VERSIONS
)"

mkdir -p /data/cache

if [ -z "$OBOT_SERVER_DSN" ]; then
  echo "OBOT_SERVER_DSN is not set. Starting PostgreSQL process..."

  # Start PostgreSQL in the background
  echo "Starting PostgreSQL server..."
  /usr/bin/docker-entrypoint.sh postgres &

  check_postgres_active
  export OBOT_SERVER_DSN="postgresql://obot:obot@localhost:5432/obot"
fi

exec tini -- obot server
