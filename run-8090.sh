#!/usr/bin/env bash
# Relaunch the Obot dev server on :8090 against ./obot.db (which holds the demo data).
#
# This manages ONLY the Go server. The SvelteKit UI dev server must be running separately on
# :5174 (see "Running the live instance" in HANDOFF-obot2-provenance.md). Open the app at
# http://localhost:8090/supervision  — NOT :5174 directly (the :8090 server proxies to it).
#
# The Go server does NOT hot-reload, so re-run this script after any Go change.
set -uo pipefail
cd "$(dirname "$0")"
[ -f keys ] || { echo "missing ./keys (OPENAI_API_KEY=... / ANTHROPIC_API_KEY=...)"; exit 1; }

# Stop whatever is serving :8090, then wait for its internal :8091 to free before relaunching.
for p in $(lsof -nP -tiTCP:8090 -sTCP:LISTEN 2>/dev/null); do kill "$p" 2>/dev/null || true; done
for _ in $(seq 1 30); do lsof -nP -iTCP:8091 -sTCP:LISTEN >/dev/null 2>&1 || break; sleep 1; done

set -a; . ./keys; set +a
OBOT_SERVER_HTTP_LISTEN_PORT=8090 \
OBOT_SERVER_URL=http://localhost:8090 \
OBOT_SERVER_DISALLOW_LOCALHOST_MCP=false \
  nohup go run main.go server --dev-mode > /tmp/obot-8090.log 2>&1 &

for i in $(seq 1 60); do
  sleep 3
  if curl -sf localhost:8090/api/healthz >/dev/null 2>&1; then
    echo "healthy on :8090 after $((i * 3))s"; exit 0
  fi
done
echo "did not become healthy within 180s; last log lines:"; tail -8 /tmp/obot-8090.log; exit 1
