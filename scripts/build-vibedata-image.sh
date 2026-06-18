#!/usr/bin/env bash
# Resolves the upstream Obot version from committed sync metadata and emits
# image tag values for use by the build-vibedata-image.yml GHA workflow.
#
# Writes to $GITHUB_OUTPUT (required) and $GITHUB_STEP_SUMMARY (optional).
# Safe to run locally: set GITHUB_OUTPUT=/dev/null if outside GHA.

set -euo pipefail

OBOT_IMAGE="${OBOT_IMAGE:-ghcr.io/accelerate-data/obot-vibedata}"
OUTPUT_FILE="${GITHUB_OUTPUT:-/dev/null}"
SUMMARY_FILE="${GITHUB_STEP_SUMMARY:-/dev/null}"

METADATA_FILE=".accelerate/upstream-sync.json"
if [ ! -f "$METADATA_FILE" ]; then
  echo "::error::${METADATA_FILE} not found — upstream-sync must run first." >&2
  exit 1
fi

UPSTREAM_VERSION="$(jq -r '.upstreamObotVersion // empty' "$METADATA_FILE")"
UPSTREAM_SHA="$(jq -r '.upstreamHeadSha // empty' "$METADATA_FILE")"

if [ -z "$UPSTREAM_VERSION" ]; then
  echo "::error::upstreamObotVersion is empty in ${METADATA_FILE}" >&2
  exit 1
fi

VERSIONED_TAG="${UPSTREAM_VERSION}-vibedata"
FORK_SHA="$(git rev-parse HEAD)"
RUN_URL="${GITHUB_SERVER_URL:-}/${GITHUB_REPOSITORY:-}/actions/runs/${GITHUB_RUN_ID:-}"

echo "upstream_version=${UPSTREAM_VERSION}"   >> "$OUTPUT_FILE"
echo "upstream_sha=${UPSTREAM_SHA}"           >> "$OUTPUT_FILE"
echo "fork_sha=${FORK_SHA}"                   >> "$OUTPUT_FILE"
echo "versioned_tag=${VERSIONED_TAG}"         >> "$OUTPUT_FILE"
echo "image=${OBOT_IMAGE}"                    >> "$OUTPUT_FILE"

{
  echo "### :whale: obot-vibedata version resolution"
  echo ""
  echo "| Signal | Value |"
  echo "| --- | --- |"
  echo "| Upstream Obot version | \`${UPSTREAM_VERSION}\` |"
  echo "| Upstream Obot SHA | \`${UPSTREAM_SHA}\` |"
  echo "| Fork Obot SHA | \`${FORK_SHA}\` |"
  echo "| Versioned tag | \`${OBOT_IMAGE}:${VERSIONED_TAG}\` |"
  echo "| Latest tag | \`${OBOT_IMAGE}:latest\` |"
  echo "| Workflow run | ${RUN_URL} |"
} >> "$SUMMARY_FILE"

echo "Resolved: ${OBOT_IMAGE}:${VERSIONED_TAG} and ${OBOT_IMAGE}:latest"
