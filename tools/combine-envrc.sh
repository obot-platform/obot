#!/bin/bash

set -e

# Combine .envrc files from providers, enterprise-providers, and encryption-bins
server_versions=""
provider_registries=""

shopt -s failglob
for file in /obot-providers/.envrc.*; do
  eval "$(grep '^export ' "$file" | sed 's/^export //')"

  if [[ -n "$OBOT_SERVER_PROVIDER_REGISTRIES" ]]; then
    provider_registries+="$OBOT_SERVER_PROVIDER_REGISTRIES,"
  fi

  if [[ -n "$OBOT_SERVER_VERSIONS" ]]; then
    server_versions+="$OBOT_SERVER_VERSIONS,"
  fi
done

cat <<EOF >/obot-providers/.envrc.providers
export OBOT_SERVER_PROVIDER_REGISTRIES="${provider_registries%,}"
export OBOT_SERVER_VERSIONS="${server_versions%,}"
EOF

rm -f /obot-providers/.envrc.providers.*
