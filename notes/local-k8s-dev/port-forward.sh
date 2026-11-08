#!/usr/bin/env bash
# Keep Obot reachable at localhost:8080, reconnecting when the pod restarts.
while true; do kubectl port-forward -n obot service/obot-obot 8080:80 >/dev/null 2>&1; sleep 2; done
