#!/usr/bin/env bash
set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../../../.." && pwd)
formal_dir="$repo_root/docs/plans/openapi-workers/formal"
statechart_ts=${STATECHART_TS:-$HOME/.agents/skills/statechart/scripts/statechart.ts}
java_bin=${JAVA:-$(command -v java || true)}
tlc_jar=${TLA2TOOLS:-/tmp/tla2tools-1.8.0.jar}
expected_sha=e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f
started_at=$(date +"%Y-%m-%dT%H:%M:%S%z")

if [[ -z "$java_bin" || ! -x "$java_bin" ]]; then
  echo "Java 11+ executable not found; set JAVA." >&2
  exit 2
fi
if [[ ! -f "$statechart_ts" ]]; then
  echo "statechart TypeScript entry not found: $statechart_ts" >&2
  exit 2
fi
if [[ ! -f "$tlc_jar" ]]; then
  curl -L --fail --silent --show-error \
    -o "$tlc_jar" \
    https://github.com/tlaplus/tlaplus/releases/download/v1.8.0/tla2tools.jar
fi
actual_sha=$(shasum -a 256 "$tlc_jar" | cut -d' ' -f1)
if [[ "$actual_sha" != "$expected_sha" ]]; then
  echo "TLC jar checksum mismatch: $actual_sha" >&2
  exit 2
fi

rm -rf \
  "$formal_dir/statechart-base" \
  "$formal_dir/statechart-chart" \
  "$formal_dir/statechart-broken-chart"

TLA2TOOLS="$tlc_jar" JAVA="$java_bin" bun "$statechart_ts" \
  "$formal_dir/ComposeLifecycle.tla" \
  --config "$formal_dir/ComposeLifecycle.cfg" \
  --view "$formal_dir/statechart-view.mjs" \
  --out "$formal_dir/statechart-base"

TLA2TOOLS="$tlc_jar" JAVA="$java_bin" bun "$statechart_ts" \
  "$formal_dir/ComposeLifecycle.tla" \
  --config "$formal_dir/ComposeLifecycleChart.cfg" \
  --view "$formal_dir/statechart-view.mjs" \
  --out "$formal_dir/statechart-chart"

set +e
TLA2TOOLS="$tlc_jar" JAVA="$java_bin" bun "$statechart_ts" \
  "$formal_dir/ComposeLifecycle.tla" \
  --config "$formal_dir/ComposeLifecycleBrokenChart.cfg" \
  --view "$formal_dir/statechart-view.mjs" \
  --out "$formal_dir/statechart-broken-chart"
broken_rc=$?
set -e
if (( broken_rc == 0 )); then
  echo "Broken model unexpectedly passed; expected the two injected failures." >&2
  exit 2
fi
if (( broken_rc != 1 )); then
  echo "Broken model failed for an unexpected tool reason (exit $broken_rc)." >&2
  exit 2
fi

base_report="$formal_dir/statechart-base/report.md"
chart_report="$formal_dir/statechart-chart/report.md"
broken_report="$formal_dir/statechart-broken-chart/report.md"

grep -qx "## Rules that hold" "$base_report"
grep -qx "TypeOK, NoDeadlock, OpenAPITrafficGated, PlannedRollRetainsCapacity, CHECK_DEADLOCK" "$base_report"
grep -q "Model checking completed. No error has been found." "$formal_dir/statechart-base/tlc.out"

grep -qx "## Rules that hold" "$chart_report"
grep -qx "TypeOK, NoDeadlock, OpenAPITrafficGated, PlannedRollRetainsCapacity, CHECK_DEADLOCK" "$chart_report"
grep -Eq 's[0-9]+: OpenAPI traffic admitted' "$formal_dir/statechart-chart/chart.mmd"
grep -Eq 's[0-9]+: Rolling upgrade, capacity retained' "$formal_dir/statechart-chart/chart.mmd"

broken_sections=$(sed -n 's/^## \(.*\) breaks$/\1/p' "$broken_report")
expected_sections=$(printf '%s\n%s\n' \
  "OpenAPITrafficGated" \
  "PlannedRollRetainsCapacity")
if [[ "$broken_sections" != "$expected_sections" ]]; then
  printf 'Unexpected broken-model findings:\n%s\n' "$broken_sections" >&2
  exit 2
fi
if grep -q '^## Rules not checked' "$broken_report"; then
  echo "Broken model left rules unchecked." >&2
  exit 2
fi
if ! grep -qx "TypeOK, NoDeadlock, CHECK_DEADLOCK" "$broken_report"; then
  echo "Broken model did not establish remaining rules." >&2
  exit 2
fi
grep -Eq 's[0-9]+: Unsafe traffic admitted' "$formal_dir/statechart-broken-chart/chart.mmd"
grep -Eq 's[0-9]+: Rolling upgrade, capacity lost' "$formal_dir/statechart-broken-chart/chart.mmd"

base_states=$(sed -n 's/^TLC reached \([0-9,]*\) states.*/\1/p' "$base_report" | head -n 1)
chart_states=$(sed -n 's/^TLC reached \([0-9,]*\) states.*/\1/p' "$chart_report" | head -n 1)
broken_states=$(sed -n 's/^TLC reached \([0-9,]*\) states.*/\1/p' "$broken_report" | head -n 1)
chart_summary=$(sed -n 's/.*The chart draws \(.*\)\.$/\1/p' "$chart_report" | head -n 1)
broken_chart_summary=$(sed -n 's/.*The chart draws \(.*\)\.$/\1/p' "$broken_report" | head -n 1)
finished_at=$(date +"%Y-%m-%dT%H:%M:%S%z")
tlc_version=$(sed -n '1s/^TLC2 Version //p' "$formal_dir/statechart-base/tlc.out")
java_version=$("$java_bin" -version 2>&1 | head -n 1)
bun_version=$(bun --version)
model_sha=$(shasum -a 256 "$formal_dir/ComposeLifecycle.tla" | cut -d' ' -f1)

cat > "$formal_dir/EVIDENCE.md" <<EOF
# F05 Compose lifecycle formal verification evidence

## Result

- Started / finished: \`$started_at\` / \`$finished_at\` (Asia/Shanghai).
- Tool: TLA+ Tools \`$tlc_version\`, jar SHA-256 \`$actual_sha\`; Bun \`$bun_version\`; \`$java_version\`.
- Full baseline config: \`docs/plans/openapi-workers/formal/ComposeLifecycle.cfg\`; \`$base_states\` distinct states; \`TypeOK\`, \`NoDeadlock\`, \`OpenAPITrafficGated\`, \`PlannedRollRetainsCapacity\`, and \`CHECK_DEADLOCK\` all hold.
- Normal-flow chart config: \`docs/plans/openapi-workers/formal/ComposeLifecycleChart.cfg\`; \`$chart_states\` states; chart: $chart_summary; the same four named checks and deadlock check hold.
- Broken chart config: \`docs/plans/openapi-workers/formal/ComposeLifecycleBrokenChart.cfg\`; \`$broken_states\` states; exactly \`OpenAPITrafficGated\` and \`PlannedRollRetainsCapacity\` break; \`TypeOK\`, \`NoDeadlock\`, and \`CHECK_DEADLOCK\` hold; no rules are unchecked.
- Model SHA-256: \`$model_sha\`.

## Commands

The project-local repeat command is:

\`\`\`bash
rtk bash .codex/skills/verify-openapi-compose/scripts/verify.sh
\`\`\`

The script downloads TLA+ Tools 1.8.0 only when \`TLA2TOOLS\` is absent, verifies the pinned SHA-256, and runs the three statechart commands above. Set \`STATECHART_TS\` or \`JAVA\` to override their default local paths.

## Expected counterexamples

- Missing route gate: from the empty deployment, \`BreakRouteGate\` admits one OpenAPI call before the shared network, PostgreSQL, service authentication, Obot, or Worker is ready, reaching \`Unsafe traffic admitted\`.
- Missing rolling guard: after one Obot and both Workers are ready, \`BreakRollGuard\` starts an Obot rollout and drains the only ready Obot replica, reaching \`Rolling upgrade, capacity lost\`.

The script compares the report's broken-rule headings with exactly those two names, rejects \`Rules not checked\`, and requires the remaining rules to hold.

## Scope and chart abstraction

- The full model explores arbitrary interleavings of network setup/removal, PostgreSQL readiness/failure, service-auth staging/trust/withdrawal, two Obot replicas, two Workers, one active call, planned one-at-a-time rolls, and an extra-failure flag. It proves the design-scope properties, not Docker Engine or Compose behavior.
- The chart model is the same TLA+ module and invariants under \`ChartConstraint\`. It follows the intended path through explicit network, PostgreSQL readiness, service-auth trust, both stacks becoming ready, admitted traffic, and a capacity-retaining rollout. It is not a substitute for the 65,706-state full baseline.
- The normal chart merges per-replica starting/ready transitions and repeated request admit/complete transitions that stay in the same visible state (36 hidden self-label steps in this run). It excludes unrelated failure/recovery interleavings; those remain covered by the full baseline.
- The broken chart is similarly bounded to the two mutation paths so statechart can complete every rule rather than stop at the full model's 100,000-state diagnostic bound.
- Not modeled: actual Docker daemon, container image contents, Compose parsing, Kubernetes, TLS handshake bytes, certificate rotation, request payload limits, leases, upstream behavior, and simultaneous double failures during a roll. A pass here does not claim E2E or deployment success.
EOF

echo "F05 formal verification passed."
echo "Evidence: $formal_dir/EVIDENCE.md"
