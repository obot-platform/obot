#!/usr/bin/env bash
# Point the kubeconfig at 127.0.0.1 (0.0.0.0 would go through the egress proxy) and
# redirect the k3d bridge's outbound 80/443 into the transparent forwarder.
set -euo pipefail
CLUSTER_NAME=obot-substrate
port=$(docker port "k3d-$CLUSTER_NAME-serverlb" 6443/tcp | head -1 | awk -F: '{print $NF}')
kubectl config set-cluster "k3d-$CLUSTER_NAME" --server="https://127.0.0.1:$port" >/dev/null
bridge="br-$(docker network inspect "k3d-$CLUSTER_NAME" -f '{{.Id}}' | cut -c1-12)"
sudo systemctl start k8s-egress
for p in 80 443; do
  rule=(PREROUTING -i "$bridge" -p tcp --dport "$p" -m addrtype ! --dst-type LOCAL ! -d 172.16.0.0/12 -j REDIRECT --to-ports 15001)
  sudo iptables -t nat -C "${rule[@]}" 2>/dev/null || sudo iptables -t nat -I "${rule[@]}"
done
sudo iptables -C INPUT -i "$bridge" -p tcp --dport 15001 -j ACCEPT 2>/dev/null || sudo iptables -I INPUT -i "$bridge" -p tcp --dport 15001 -j ACCEPT
echo "egress redirected for $bridge"
# Trust the discobox MITM CA in pods: copy this box's bundle into the node; the
# MutatingAdmissionPolicy in ca-policy.yaml mounts it over the system bundle.
docker cp -L "$SSL_CERT_FILE" "k3d-$CLUSTER_NAME-server-0:/etc/discobox-ca-bundle.crt"
kubectl apply -f "$(dirname "$0")/ca-policy.yaml" >/dev/null
kubectl apply -f "$(dirname "$0")/coredns-custom.yaml" >/dev/null
# Substrate composes actor root filesystems with overlayfs under /var/lib/ate, which can't use
# the node container's own overlay root as an upperdir. Back it with the k3s data volume (ext4),
# then restart the pods that hold /var/lib/ate.
N="k3d-$CLUSTER_NAME-server-0"
if [[ "$(docker exec "$N" stat -f -c %T /var/lib/ate 2>/dev/null)" == overlayfs ]]; then
  docker exec "$N" sh -c 'mkdir -p /var/lib/rancher/k3s/ate-data /var/lib/ate && cp -a /var/lib/ate/. /var/lib/rancher/k3s/ate-data/ 2>/dev/null; mount --bind /var/lib/rancher/k3s/ate-data /var/lib/ate'
  kubectl -n ate-system delete pod -l app=atelet --wait=false >/dev/null 2>&1 || true
  kubectl -n obot-agents delete pod --all --wait=false >/dev/null 2>&1 || true
fi
