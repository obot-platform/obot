#!/usr/bin/env bash
# Verifies .accelerate/upstream-sync.json (which drives the vibedata image tag
# via scripts/build-vibedata-image.sh) is well-formed and not stale.
#
# Drift rule: the recorded upstreamObotVersion must NOT be behind the highest
# stable upstream release tag (vX.Y.Z) that is actually merged into HEAD. This
# fires precisely when a branch pulls in a newer upstream tag than the metadata
# records (i.e. a sync that forgot to bump the metadata) and is a no-op for
# ordinary feature branches, which don't advance the merged upstream tag.
#
# Requires upstream version tags (refs/tags/v*) to be present locally — the CI
# job fetches them from upstream before invoking this script.
#
# Safe to run locally: bash scripts/verify-sync-metadata.sh

set -euo pipefail

META=".accelerate/upstream-sync.json"

fail() {
  echo "::error::$*" >&2
  echo "FAIL: $*"
  exit 1
}

[ -f "$META" ] || fail "${META} not found"
jq -e . "$META" >/dev/null 2>&1 || fail "${META} is not valid JSON"

VERSION="$(jq -r '.upstreamObotVersion // empty' "$META")"
HEAD_SHA="$(jq -r '.upstreamHeadSha // empty' "$META")"
MERGE_BASE="$(jq -r '.mergeBaseSha // empty' "$META")"

[ -n "$VERSION" ] || fail "upstreamObotVersion is empty in ${META}"
printf '%s' "$VERSION"    | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$' \
  || fail "upstreamObotVersion '${VERSION}' is not a stable vX.Y.Z tag"
printf '%s' "$HEAD_SHA"   | grep -qE '^[0-9a-f]{40}$' \
  || fail "upstreamHeadSha '${HEAD_SHA}' is not a 40-character commit sha"
printf '%s' "$MERGE_BASE" | grep -qE '^[0-9a-f]{40}$' \
  || fail "mergeBaseSha '${MERGE_BASE}' is not a 40-character commit sha"

# Highest stable vX.Y.Z tag whose commit is an ancestor of HEAD (excludes RC and
# malformed tags such as v.0.23.2-rc1).
ANCESTOR_LATEST="$(git tag --merged HEAD --sort=-v:refname 2>/dev/null \
  | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || true)"

if [ -n "$ANCESTOR_LATEST" ] && [ "$ANCESTOR_LATEST" != "$VERSION" ]; then
  HIGHER="$(printf '%s\n%s\n' "$VERSION" "$ANCESTOR_LATEST" | sort -V | tail -1)"
  if [ "$HIGHER" = "$ANCESTOR_LATEST" ]; then
    fail "image version metadata is stale: HEAD contains upstream release ${ANCESTOR_LATEST}, but ${META} records ${VERSION}. Bump upstreamObotVersion (and the SHAs) to ${ANCESTOR_LATEST} — or run the Upstream Sync workflow, which writes it automatically."
  fi
fi

echo "OK: upstreamObotVersion=${VERSION}; highest merged upstream tag=${ANCESTOR_LATEST:-none}"
