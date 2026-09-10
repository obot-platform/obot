#!/usr/bin/env bash
set -euo pipefail

rendered=$(helm template obot chart --set dev.useEmbeddedDb=true --show-only templates/internal-configmap.yaml)
[[ "$rendered" == *'OBOT_SERVER_MODEL_PROXY_URL: "https://model-service.obot.ai"'* ]]

rendered=$(helm template obot chart --set dev.useEmbeddedDb=true --set-string config.OBOT_SERVER_MODEL_PROXY_URL= --show-only templates/internal-configmap.yaml)
[[ "$rendered" == *'OBOT_SERVER_MODEL_PROXY_URL: ""'* ]]

rendered=$(helm template obot chart --set dev.useEmbeddedDb=true --set-string config.OBOT_SERVER_MODEL_PROXY_URL=https://custom.example/prefix --show-only templates/internal-configmap.yaml)
[[ "$rendered" == *'OBOT_SERVER_MODEL_PROXY_URL: "https://custom.example/prefix"'* ]]

rendered=$(helm template obot chart --set dev.useEmbeddedDb=true --set config.OBOT_SERVER_MODEL_PROXY_URL=null --show-only templates/internal-configmap.yaml)
[[ "$rendered" != *'OBOT_SERVER_MODEL_PROXY_URL:'* ]]
