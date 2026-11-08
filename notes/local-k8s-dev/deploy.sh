#!/usr/bin/env bash
# Build bin/obot (with UI) unless SKIP_BUILD=1, bake it into a dev image, import it into
# k3d, and helm upgrade the release onto it.
set -euo pipefail
REPO=/workspace/obot
HERE=$(cd "$(dirname "$0")" && pwd)
CLUSTER_NAME=obot-substrate
cd "$REPO"
eval "$(mise env -s bash)"
if [[ "${SKIP_BUILD:-}" != 1 ]]; then
  if [[ "${SKIP_UI:-}" != 1 ]]; then make all; else make build; fi
fi
TAG="dev-$(date +%Y%m%d%H%M%S)"
ctx=$(mktemp -d); cp bin/obot "$ctx/"
docker build -q -t "obot:$TAG" -f "$HERE/Dockerfile.dev" "$ctx" >/dev/null
rm -rf "$ctx"
k3d image import "obot:$TAG" --cluster "$CLUSTER_NAME" >/dev/null
# The cluster has its own copy now; keep no dev images (or the base) in local Docker.
docker images --format '{{.Repository}}:{{.Tag}}' | grep '^obot:dev-' | xargs -r docker rmi >/dev/null 2>&1 || true
docker image prune -af >/dev/null 2>&1 || true
helm upgrade --install obot ./chart \
  --kube-context "k3d-$CLUSTER_NAME" --create-namespace --namespace obot \
  -f "$HERE/values.yaml" \
  --set-string image.tag="$TAG" \
  --wait --wait-for-jobs --timeout 15m
# Drop the previous dev image from the node, by its exact tag, once the rollout is done.
for old in $(docker exec "k3d-$CLUSTER_NAME-server-0" crictl images -o json | jq -r '.images[] | select(any(.repoTags[]; startswith("docker.io/library/obot:dev-")) and (any(.repoTags[]; . == "docker.io/library/obot:'"$TAG"'") | not)) | .id'); do
  docker exec "k3d-$CLUSTER_NAME-server-0" crictl rmi "$old" >/dev/null 2>&1 || true
done
echo "deployed obot:$TAG"
