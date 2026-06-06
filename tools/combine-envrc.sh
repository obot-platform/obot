#!/bin/bash

set -e
# Combine .envrc files from providers, enterprise-providers, and encryption-bins
PROVIDERS_DIR=/obot-providers
ENV_FILES=$(ls "$PROVIDERS_DIR"/.envrc.* 2>/dev/null)

server_versions=""
provider_registries=""

for file in ${ENV_FILES[@]}; do
  eval "$(grep '^export ' "$file" | sed 's/^export //')"

  if [[ -n "$OBOT_SERVER_PROVIDER_REGISTRIES" ]]; then
    provider_registries+="$OBOT_SERVER_PROVIDER_REGISTRIES,"
  fi

  if [[ -n "$OBOT_SERVER_VERSIONS" ]]; then
    server_versions+="$OBOT_SERVER_VERSIONS,"
  fi
done

OBOT_SERVER_VERSIONS="${server_versions%,}"
OBOT_SERVER_PROVIDER_REGISTRIES="${provider_registries%,}"

cat <<EOF >/obot-providers/.envrc.providers
export OBOT_SERVER_PROVIDER_REGISTRIES="${OBOT_SERVER_PROVIDER_REGISTRIES}"
export OBOT_SERVER_VERSIONS="${OBOT_SERVER_VERSIONS}"
EOF

rm -f /obot-providers/.envrc.providers.*
