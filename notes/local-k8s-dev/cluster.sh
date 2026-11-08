#!/usr/bin/env bash
# Recreate the obot-substrate k3d cluster and re-point egress at its bridge.
set -euo pipefail
CLUSTER_NAME=obot-substrate
k3d cluster delete "$CLUSTER_NAME" >/dev/null 2>&1 || true
KUBELET_GATES='--kubelet-arg=feature-gates=ClusterTrustBundle=true,ClusterTrustBundleProjection=true,PodCertificateRequest=true'
k3d cluster create "$CLUSTER_NAME" \
  --image rancher/k3s:v1.36.4-k3s1 --servers 1 --agents 0 \
  --k3s-arg '--kube-apiserver-arg=runtime-config=certificates.k8s.io/v1beta1=true@server:*' \
  --k3s-arg '--kube-apiserver-arg=feature-gates=ClusterTrustBundle=true,PodCertificateRequest=true@server:*' \
  --k3s-arg "$KUBELET_GATES@server:*" \
  --k3s-arg '--kubelet-arg=eviction-hard=imagefs.available<1%,nodefs.available<1%@server:*' \
  --k3s-arg '--kubelet-arg=eviction-minimum-reclaim=imagefs.available=1%,nodefs.available=1%@server:*' \
  --k3s-arg '--kubelet-arg=image-gc-high-threshold=99@server:*' \
  --k3s-arg '--kubelet-arg=image-gc-low-threshold=98@server:*' \
  --k3s-arg '--disable=traefik@server:*' \
  --wait
"$(dirname "$0")/egress-up.sh"
