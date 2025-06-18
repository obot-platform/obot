#! /bin/bash

export OBOT_SERVER_TOOL_REGISTRIES="github.com/obot-platform/tools"
echo "Starting obot server..."
./bin/obot server > ./obot.log 2>&1 &

URL="http://localhost:8080/api/healthz"
TIMEOUT=300
INTERVAL=5
MAX_RETRIES=$((TIMEOUT / INTERVAL))

echo "Waiting for $URL to return OK..."

for ((i=1; i<=MAX_RETRIES; i++)); do
  response=$(curl -s -o /dev/null -w "%{http_code}" "$URL" 2>/dev/null)
  
  if [ "$response" = "200" ]; then
    health_content=$(curl -s "$URL")
    if [[ "$health_content" == *"ok"* ]]; then
      echo "✅ Health check passed! Response: $health_content"
      go test ./tests/integration/... -v
      exit 0
    else
      echo "⚠️  Got HTTP 200 but unexpected response: $health_content"
    fi
  fi

  echo "Attempt $i/$MAX_RETRIES: Service not ready (HTTP $response). Retrying in $INTERVAL seconds..."
  sleep "$INTERVAL"
done

echo "❌ Timeout reached! Service at $URL did not return OK within $TIMEOUT seconds"
tail -n 100 ./obot.log
exit 1

