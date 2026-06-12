#!/usr/bin/env bash
set -euo pipefail

BASE_REF="${1:-origin/main}"
ALLOWED=(
  "pkg/oidcjwt/"
  "pkg/services/config.go"
  "chart/values.yaml"
  "go.mod"
  "go.sum"
  "docs/design/oidc-jwt-authn/"
  "docs/plans/2026-06-12-oidc-jwt-authn.md"
  "docs/studio/CHANGES.md"
  "scripts/check-upstream-touchpoints.sh"
)

changed=$(git diff --name-only "$BASE_REF"...HEAD)
unexpected=()
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  ok=0
  for a in "${ALLOWED[@]}"; do
    if [[ "$f" == "$a"* ]]; then
      ok=1
      break
    fi
  done
  if [[ $ok -eq 0 ]]; then
    unexpected+=("$f")
  fi
done <<< "$changed"

if [[ ${#unexpected[@]} -gt 0 ]]; then
  echo "Unexpected upstream touchpoints:"
  printf '  %s\n' "${unexpected[@]}"
  exit 1
fi

echo "OK - all changes within the allowed set."
