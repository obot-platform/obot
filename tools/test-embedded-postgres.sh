#!/usr/bin/env bash
# Run against a locally built final Obot image. All data and containers are disposable.
set -euo pipefail

image=${1:?Usage: test-embedded-postgres.sh IMAGE}
# v0.27.0, AMD64. Override for another architecture or a different PG17 baseline.
old_image=${OLD_OBOT_IMAGE:-ghcr.io/obot-platform/obot:v0.27.0@sha256:3bf698365660c4a8001f378f1a198464c6220a48c9085e619921b89560074450}
work=$(mktemp -d /tmp/obot-embedded-postgres-XXXXXX)
prefix=${work##*/}
mkdir "$work/logs"
containers=()
volumes=()

cleanup() {
  local status=$? container
  for container in "${containers[@]}"; do
    docker logs "$container" >"$work/logs/$container.log" 2>&1 || true
    docker rm -f -v "$container" >/dev/null 2>&1 || true
  done
  for volume in "${volumes[@]}"; do
    docker volume rm "$volume" >/dev/null 2>&1 || true
  done
  docker network rm "$prefix" >/dev/null 2>&1 || true
  echo "Embedded PostgreSQL test logs: $work/logs"
  exit "$status"
}
trap cleanup EXIT

fail() { echo "$*" >&2; exit 1; }
new_volume() {
  volumes+=("$prefix-$1")
  docker volume create "$prefix-$1" >/dev/null
}
sql() {
  local container=$1 db=$2
  shift 2
  docker exec -i "$container" psql -X -v ON_ERROR_STOP=1 -U obot -d "$db" -At "$@"
}
wait_db() {
  local container=$1
  for ((i=0; i<90; i++)); do
    # The entrypoint's temporary initdb server can accept connections too. Wait for
    # the final postgres process (PID 1) before dumping or restoring.
    if docker exec "$container" sh -c 'test "$(head -n 1 "$PGDATA/postmaster.pid")" = 1 && pg_isready -q -U obot -d postgres' 2>/dev/null; then
      return
    fi
    sleep 1
  done
  fail "PostgreSQL did not start in $container"
}
start_db() {
  local name=$1 db_image=$2 volume=$3 db=$4
  containers+=("$prefix-$name")
  docker run -d --name "$prefix-$name" --network "$prefix" \
    -v "$prefix-$volume:/data" -e PGDATA=/data/postgresql \
    -e POSTGRES_USER=obot -e POSTGRES_PASSWORD=obot -e POSTGRES_DB="$db" \
    --entrypoint /usr/bin/docker-entrypoint.sh "$db_image" postgres >/dev/null
  wait_db "$prefix-$name"
}
wait_app() {
  local name=$1
  for ((i=0; i<180; i++)); do
    if docker exec "$name" python3 -c 'import urllib.request; urllib.request.urlopen("http://localhost:8080/api/healthz", timeout=2)' 2>/dev/null; then
      return
    fi
    [[ $(docker inspect -f '{{.State.Running}}' "$name") == true ]] || fail "Obot exited in $name"
    sleep 1
  done
  fail "Obot did not become healthy in $name"
}
start_app() {
  local name=$1 volume=$2
  shift 2
  containers+=("$prefix-$name")
  docker run -d --name "$prefix-$name" --network "$prefix" \
    -v "$prefix-$volume:/data" -e OBOT_SERVER_DISABLE_UPDATE_CHECK=true \
    -e "DOCKER_HOST=tcp://$prefix-docker:2375" \
    "$@" "$image" >/dev/null
  wait_app "$prefix-$name"
}
cluster_hash() {
  docker run --rm --network none --entrypoint sh -v "$prefix-old:/data:ro" "$image" \
    -c 'find /data/postgresql -type f -exec sha256sum {} + | sort | sha256sum'
}
reject_version() {
  local name=$1 volume=$2 pgdata=$3
  containers+=("$prefix-$name")
  docker run -d --name "$prefix-$name" --network none \
    -v "$prefix-$volume:/data" -e PGDATA="$pgdata" "$image" >/dev/null
  [[ $(timeout 10 docker wait "$prefix-$name") == 1 ]] || fail "Expected prompt failure for $name"
  docker logs "$prefix-$name" >"$work/logs/$name.log" 2>&1
  grep -Fq "The embedded database at $pgdata" "$work/logs/$name.log"
  grep -Fq 'uses PostgreSQL 18' "$work/logs/$name.log"
  grep -Fq 'https://docs.obot.ai/next/operations/embedded-postgresql-upgrade' "$work/logs/$name.log"
  if grep -Eq 'Waiting for PostgreSQL|Starting PostgreSQL server' "$work/logs/$name.log"; then
    fail "Incompatible cluster reached PostgreSQL startup"
  fi
}
check_restore_log() {
  local log=$1 errors
  errors=$(grep -E '(ERROR|FATAL|PANIC):' "$log" || true)
  [[ "$errors" == 'ERROR:  role "obot" already exists' ]]
}

docker network create "$prefix" >/dev/null
docker pull "$old_image" >/dev/null

# Like tests/e2e/harness/FakeDocker, report an empty Docker backend and refuse
# mutations. Never give a test Obot access to the host's real Docker daemon.
cat >"$work/docker-api.py" <<'PY'
import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

class Handler(BaseHTTPRequestHandler):
    def do_HEAD(self):
        self.do_GET()

    def do_GET(self):
        path = self.path.split('?')[0]
        status, data = 200, {}
        if path.endswith('/_ping'):
            data = 'OK'
        elif path.endswith(('/containers/json', '/networks', '/volumes')):
            data = []
        elif path.endswith('/version'):
            data = {'ApiVersion': '1.43', 'Version': 'test'}
        elif path.endswith('/info'):
            data = {'OSType': 'linux'}
        elif not path.endswith('/events'):
            status, data = 404, {'message': 'No such object (test docker)'}
        self.send_response(status)
        self.send_header('Api-Version', '1.43')
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        if path.endswith('/events'):
            time.sleep(900)
        elif self.command != 'HEAD':
            self.wfile.write((data if isinstance(data, str) else json.dumps(data)).encode())

ThreadingHTTPServer(('0.0.0.0', 2375), Handler).serve_forever()
PY
containers+=("$prefix-docker")
docker run -d --name "$prefix-docker" --network "$prefix" \
  -v "$work/docker-api.py:/tmp/docker-api.py:ro" --entrypoint python3 \
  "$image" /tmp/docker-api.py >/dev/null

echo 'Checking image packages and entrypoint'
docker run --rm --network none --entrypoint sh "$image" -ec '
  postgres --version | grep -E "PostgreSQL\) 18\."
  pg_dumpall --version | grep -E "PostgreSQL\) 18\."
  psql --version | grep -E "PostgreSQL\) 18\."
  test -x /usr/bin/docker-entrypoint.sh
  apk info -e postgresql-18 postgresql-18-client postgresql-18-contrib postgresql-18-oci-entrypoint
  if apk info | grep -E "^(postgresql|ecpg|libecpg|libpq)-17($|-)"; then exit 1; fi
'

echo 'Checking fresh application startup and persisted restart'
new_volume fresh
start_app fresh fresh
[[ $(sql "$prefix-fresh" postgres -c 'SHOW server_version_num') == 18* ]]
[[ $(sql "$prefix-fresh" postgres -c "SELECT count(*) FROM pg_database WHERE datname = 'obot'") == 1 ]]
# Fresh installs share Obot's database; only restored legacy clusters retain the old gateway database.
[[ $(sql "$prefix-fresh" postgres -c "SELECT count(*) FROM pg_database WHERE datname = 'obot_llm_gateway'") == 0 ]]
sql "$prefix-fresh" obot -c "CREATE TABLE embedded_upgrade_marker (value text); INSERT INTO embedded_upgrade_marker VALUES ('persisted');" >/dev/null
docker restart -t 30 "$prefix-fresh" >/dev/null
wait_app "$prefix-fresh"
[[ $(sql "$prefix-fresh" obot -c 'SELECT value FROM embedded_upgrade_marker') == persisted ]]
docker stop -t 30 "$prefix-fresh" >/dev/null

echo 'Preparing a real PG17 cluster with two databases, a role, grants and sequences'
new_volume old
start_db old "$old_image" old obot
[[ $(sql "$prefix-old" postgres -c 'SHOW server_version_num') == 17* ]]
sql "$prefix-old" postgres <<'SQL' >/dev/null
CREATE DATABASE obot_llm_gateway OWNER obot;
CREATE ROLE migration_reader;
SQL
for db in obot obot_llm_gateway; do
  sql "$prefix-old" "$db" <<'SQL' >/dev/null
CREATE TABLE migration_records (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, value text);
INSERT INTO migration_records (value) VALUES ('retained'), ('unicode: 日本語');
GRANT SELECT ON migration_records TO migration_reader;
SQL
done
docker exec "$prefix-old" sh -c 'printf "keep me\n" > /data/keep-me'
docker exec "$prefix-old" pg_dumpall -U obot --quote-all-identifiers >"$work/backup.sql"
test -s "$work/backup.sql"
docker stop -t 30 "$prefix-old" >/dev/null
before=$(cluster_hash)
reject_version old-rejected old /data/postgresql
[[ $(cluster_hash) == "$before" ]] || fail 'Incompatible PGDATA was modified'
docker start "$prefix-old" >/dev/null
wait_db "$prefix-old"
[[ $(sql "$prefix-old" obot -c 'SELECT count(*) FROM migration_records') == 2 ]]
docker stop -t 30 "$prefix-old" >/dev/null

echo 'Checking custom PGDATA and malformed or unsupported version markers'
new_volume invalid
for version in 16 broken ''; do
  docker run --rm --network none --entrypoint sh -v "$prefix-invalid:/data" "$image" \
    -c 'mkdir -p "/data/custom db"; printf "%s\n" "$1" > "/data/custom db/PG_VERSION"' sh "$version"
  reject_version "invalid-${version:-empty}" invalid '/data/custom db'
done
docker run --rm --network none --entrypoint sh -v "$prefix-invalid:/data" "$image" \
  -c 'printf "17\n" > "/data/custom db/PG_VERSION"'
docker run --rm --network none -v "$prefix-invalid:/data" -e 'PGDATA=/data/custom db' \
  "$image" tunnel --help >"$work/logs/tunnel.log" 2>&1
if grep -q 'embedded database' "$work/logs/tunnel.log"; then
  fail 'Tunnel command checked the embedded database'
fi

echo 'Restoring into PG18 with Obot stopped, following the documented procedure'
docker run --rm --network none --entrypoint sh -v "$prefix-old:/data" "$image" \
  -ec 'test ! -e /data/postgresql-17-backup; mv /data/postgresql /data/postgresql-17-backup'
start_db restored "$image" old postgres
[[ $(sql "$prefix-restored" postgres -c "SELECT count(*) FROM pg_database WHERE datname = 'obot'") == 0 ]]
docker exec -i "$prefix-restored" psql -X -U obot -d postgres \
  -v ON_ERROR_STOP=off -v VERBOSITY=terse <"$work/backup.sql" \
  >"$work/logs/restore.out" 2>"$work/logs/restore.log"
check_restore_log "$work/logs/restore.log" || fail 'Unexpected restore error; see restore.log'
# psql can exit zero after SQL errors. Prove our acceptance check rejects those.
printf '%s\n' 'ERROR:  role "obot" already exists' 'ERROR:  unexpected failure' >"$work/logs/rejected-restore.log"
if check_restore_log "$work/logs/rejected-restore.log"; then
  fail 'Unexpected restore errors were accepted'
fi
for db in obot obot_llm_gateway; do
  [[ $(sql "$prefix-restored" "$db" -c 'SELECT count(*) FROM migration_records') == 2 ]]
  [[ $(sql "$prefix-restored" "$db" -c "SELECT value FROM migration_records WHERE id = 2") == 'unicode: 日本語' ]]
  [[ $(sql "$prefix-restored" "$db" -c "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE relname = 'migration_records'") == obot ]]
  [[ $(sql "$prefix-restored" "$db" -c "SELECT has_table_privilege('migration_reader', 'migration_records', 'SELECT')") == t ]]
  [[ $(sql "$prefix-restored" "$db" -c "INSERT INTO migration_records (value) VALUES ('new') RETURNING id" | head -1) == 3 ]]
done
docker exec "$prefix-restored" vacuumdb -U obot --all --analyze >/dev/null
[[ $(docker exec "$prefix-restored" cat /data/keep-me) == 'keep me' ]]
[[ $(docker exec "$prefix-restored" cat /data/postgresql-17-backup/PG_VERSION) == 17 ]]
docker stop -t 30 "$prefix-restored" >/dev/null
start_app migrated old
[[ $(sql "$prefix-migrated" obot_llm_gateway -c 'SELECT count(*) FROM migration_records') == 3 ]]
docker stop -t 30 "$prefix-migrated" >/dev/null

echo 'Checking external PG17 startup ignores an old local PGDATA'
new_volume external
start_db external-db "$old_image" external obot
start_app external-app invalid -e 'PGDATA=/data/custom db' \
  -e "OBOT_SERVER_DSN=postgresql://obot:obot@$prefix-external-db:5432/obot"
docker logs "$prefix-external-app" >"$work/logs/external.log" 2>&1
if grep -q 'Starting PostgreSQL server' "$work/logs/external.log"; then
  fail 'External DSN started the embedded database'
fi

echo 'Embedded PostgreSQL startup and migration checks passed'
