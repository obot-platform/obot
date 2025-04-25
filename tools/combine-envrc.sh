#!/bin/bash

set -e
# Combine .envrc files from tools, enterprise-tools, and provider
TOOLS_DIR=/obot-tools
ENV_FILES=$(ls $TOOLS_DIR/.envrc.* 2>/dev/null)

remap_entries=""
server_versions=""
tool_registries=""

for file in ${ENV_FILES[@]}; do
  eval "$(grep '^export ' "$file" | sed 's/^export //')"

  # Split and accumulate GPTSCRIPT_TOOL_REMAP entries
  if [[ -n "$GPTSCRIPT_TOOL_REMAP" ]]; then
    remap_entries+="$GPTSCRIPT_TOOL_REMAP,"
  fi

  if [[ -n "$OBOT_SERVER_TOOL_REGISTRIES" ]]; then
    tool_registries+="$OBOT_SERVER_TOOL_REGISTRIES,"
  fi

  if [[ -n "$OBOT_SERVER_VERSIONS" ]]; then
    server_versions+="$OBOT_SERVER_VERSIONS,"
  fi
done

GPTSCRIPT_TOOL_REMAP="${remap_entries%,}"
OBOT_SERVER_VERSIONS="${server_versions%,}"
OBOT_SERVER_TOOL_REGISTRIES="${tool_registries%,}"

cat <<EOF >/obot-tools/.envrc.tools
export GPTSCRIPT_SYSTEM_TOOLS_DIR=/obot-tools/
export TOOLS_VENV_BIN=/obot-tools/venv/bin
export GPTSCRIPT_TOOL_REMAP="${GPTSCRIPT_TOOL_REMAP}"
export OBOT_SERVER_TOOL_REGISTRIES="${OBOT_SERVER_TOOL_REGISTRIES}"
export OBOT_SERVER_VERSIONS="${OBOT_SERVER_VERSIONS}"
EOF

rm -f /obot-tools/.envrc.tools.*