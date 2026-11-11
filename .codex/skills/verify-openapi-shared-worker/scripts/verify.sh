#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
statechart=""
downloaded_tlc=""
expected_tlc_sha256="e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f"

cleanup() {
    if [[ -n "${downloaded_tlc}" ]]; then
        rm -f "${downloaded_tlc}"
    fi
}
trap cleanup EXIT

for candidate in \
    "${HOME}/.agents/skills/statechart/scripts/statechart.ts" \
    "${HOME}/.codex/skills/statechart/scripts/statechart.ts"
do
    if [[ -x "${candidate}" || -f "${candidate}" ]]; then
        statechart="${candidate}"
        break
    fi
done

if [[ -z "${statechart}" ]]; then
    echo "statechart skill script not found" >&2
    exit 2
fi

if [[ -z "${TLA2TOOLS:-}" ]]; then
    downloaded_tlc="$(mktemp -t tla2tools-v1.8.0.XXXXXX.jar)"
    curl -fL --retry 3 \
        -o "${downloaded_tlc}" \
        https://github.com/tlaplus/tlaplus/releases/download/v1.8.0/tla2tools.jar
    actual_tlc_sha256="$(shasum -a 256 "${downloaded_tlc}" | awk '{print $1}')"
    if [[ "${actual_tlc_sha256}" != "${expected_tlc_sha256}" ]]; then
        echo "TLA+ Tools checksum mismatch: ${actual_tlc_sha256}" >&2
        exit 2
    fi
    export TLA2TOOLS="${downloaded_tlc}"
fi

rm -rf \
    "${root}/docs/plans/openapi-workers/formal/evidence/base" \
    "${root}/docs/plans/openapi-workers/formal/evidence/broken"
mkdir -p \
    "${root}/docs/plans/openapi-workers/formal/evidence/base" \
    "${root}/docs/plans/openapi-workers/formal/evidence/broken"

cd "${root}/docs/plans/openapi-workers/formal"

set +e
node "${statechart}" SnapshotFencing.tla \
    --config SnapshotFencingBroken.cfg \
    --view snapshot-fencing.view.mjs \
    --out evidence/broken
broken_status=$?

node "${statechart}" SnapshotFencing.tla \
    --config SnapshotFencing.cfg \
    --view snapshot-fencing.view.mjs \
    --out evidence/base
base_status=$?
set -e

echo "broken_status=${broken_status}"
echo "base_status=${base_status}"

if (( base_status != 0 )); then
    echo "Expected the guarded configuration to pass." >&2
    exit 1
fi
if (( broken_status != 1 )); then
    echo "Expected the deliberately broken configuration to fail an invariant." >&2
    exit 1
fi

broken_breaks="$(grep -E '^## .* breaks$' \
    "${root}/docs/plans/openapi-workers/formal/evidence/broken/report.md" || true)"

for required_break in \
    '## ExecutionEnteredAtMostOnce breaks' \
    '## UpstreamRequiresOneSnapshotAndOneFence breaks'
do
    if ! grep -Fqx "${required_break}" <<<"${broken_breaks}"; then
        echo "Broken model did not report ${required_break}." >&2
        exit 1
    fi
done

while IFS= read -r reported_break; do
    case "${reported_break}" in
        '## ExecutionEnteredAtMostOnce breaks'|\
        '## UpstreamRequiresOneSnapshotAndOneFence breaks'|\
        '## NoRetryAfterUncertainOutcome breaks')
            ;;
        *)
            echo "Broken model reported an unexpected violation: ${reported_break}" >&2
            exit 1
            ;;
    esac
done <<<"${broken_breaks}"

if grep -Fq '## Rules not checked' \
    "${root}/docs/plans/openapi-workers/formal/evidence/broken/report.md"; then
    echo 'Broken model did not check every rule.' >&2
    exit 1
fi

for required_rule in \
    TypeOK \
    SnapshotSuppliedAtMostOnce \
    ExecutionEnteredAtMostOnce \
    UpstreamAttemptedAtMostOnce \
    UpstreamRequiresOneSnapshotAndOneFence \
    SnapshotRequiredNeverTouchesUpstream \
    NoRetryAfterUncertainOutcome \
    NoDeadlock
do
    if ! grep -Fq "${required_rule}" \
        "${root}/docs/plans/openapi-workers/formal/evidence/base/report.md"; then
        echo "Guarded model did not report ${required_rule} as holding." >&2
        exit 1
    fi
done
