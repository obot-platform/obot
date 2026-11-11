#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
FORMAL="$ROOT/docs/plans/openapi-workers/formal"
STATECHART="${STATECHART:-$HOME/.agents/skills/statechart/scripts/statechart}"
JAVA_BIN="${JAVA:-java}"
TLA_URL="https://github.com/tlaplus/tlaplus/releases/download/v1.8.0/tla2tools.jar"
TLA_SHA256="e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f"
SCRATCH_JAR=""

cleanup() {
  if [[ -n "${FORMAL:-}" && -d "$FORMAL" ]]; then
    rm -f "$FORMAL"/ExecutionBudgetLeases_TTrace_*.tla
    rm -f "$FORMAL"/ExecutionBudgetLeases_TTrace_*.bin
    rm -rf "$FORMAL/states"
  fi
  if [[ -n "${ROOT:-}" && -d "$ROOT" ]]; then
    rm -rf "$ROOT/states"
  fi
  if [[ -n "$SCRATCH_JAR" && -f "$SCRATCH_JAR" ]]; then
    rm -f "$SCRATCH_JAR"
  fi
}
trap cleanup EXIT

fail() {
  echo "verify-openapi-execution-budgets: $*" >&2
  exit 1
}

if [[ ! -x "$STATECHART" ]]; then
  fail "statechart executable not found at $STATECHART"
fi
if ! command -v "$JAVA_BIN" >/dev/null 2>&1; then
  fail "Java executable not found: $JAVA_BIN"
fi

if [[ -z "${TLA2TOOLS:-}" ]]; then
  SCRATCH_JAR="$(mktemp "${TMPDIR:-/tmp}/verify-openapi-budgets-tla2tools.XXXXXX.jar")"
  curl -fsSL "$TLA_URL" -o "$SCRATCH_JAR"
  export TLA2TOOLS="$SCRATCH_JAR"
fi
[[ -f "$TLA2TOOLS" ]] || fail "TLA2TOOLS does not name a file: $TLA2TOOLS"
actual_sha="$(shasum -a 256 "$TLA2TOOLS" | awk '{print $1}')"
[[ "$actual_sha" == "$TLA_SHA256" ]] ||
  fail "TLA+ jar hash mismatch: expected $TLA_SHA256, got $actual_sha"

started_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
runs="$FORMAL/runs/latest"
mkdir -p "$runs"
rm -f "$runs"/{normal.out,broken-early-release.out,broken-stale-start.out,java.txt,times.txt}
rm -rf "$FORMAL"/{statechart-chart,statechart-broken-early,statechart-broken-stale}
rm -f "$FORMAL/RUN.md"
rm -f "$FORMAL"/ExecutionBudgetLeases_TTrace_*.tla
rm -f "$FORMAL"/ExecutionBudgetLeases_TTrace_*.bin
rm -rf "$FORMAL/states"
rm -rf "$ROOT/states"

"$JAVA_BIN" -version 2>&1 | head -3 >"$runs/java.txt"

run_full_tlc() {
  local name="$1" config="$2" expected="$3"
  local output="$runs/$name.out"
  date -u "+$name start %Y-%m-%dT%H:%M:%SZ" | tee -a "$runs/times.txt"
  set +e
  "$JAVA_BIN" -XX:+UseParallelGC -cp "$TLA2TOOLS" tlc2.TLC \
    -config "$FORMAL/$config" "$FORMAL/ExecutionBudgetLeases.tla" >"$output"
  local status=$?
  set -e
  date -u "+$name finish %Y-%m-%dT%H:%M:%SZ" | tee -a "$runs/times.txt"
  [[ "$status" == "$expected" ]] ||
    fail "full $name exited $status; expected $expected (see $output)"
}

run_statechart() {
  local name="$1" spec="$2" config="$3" expected="$4"
  local out="$FORMAL/$name"
  set +e
  "$STATECHART" "$FORMAL/$spec" \
    --config "$FORMAL/$config" \
    --view "$FORMAL/lease-chart.view.mjs" \
    --out "$out" >"$runs/$name.console.txt" 2>&1
  local status=$?
  set -e
  [[ "$status" == "$expected" ]] ||
    fail "chart $name exited $status; expected $expected (see $runs/$name.console.txt)"
}

run_full_tlc normal ExecutionBudgetLeases.cfg 0
run_full_tlc broken-early-release ExecutionBudgetLeasesBrokenEarlyRelease.cfg 12
run_full_tlc broken-stale-start ExecutionBudgetLeasesBrokenStaleStart.cfg 12

run_statechart statechart-chart \
  OpenAPIBudgetLeasesChart.tla OpenAPIBudgetLeasesChart.cfg 0
run_statechart statechart-broken-early \
  OpenAPIBudgetLeasesChartBrokenEarlyRelease.tla \
  OpenAPIBudgetLeasesChartBrokenEarlyRelease.cfg 1
run_statechart statechart-broken-stale \
  OpenAPIBudgetLeasesChartBrokenStaleStart.tla \
  OpenAPIBudgetLeasesChartBrokenStaleStart.cfg 1

for output in "$runs/normal.out" \
              "$runs/broken-early-release.out" \
              "$runs/broken-stale-start.out"; do
  grep -Fq 'TLC2 Version 2026.11.06.222357 (rev: d8fb231)' "$output" ||
    fail "$output did not use the pinned TLC build"
done

normal="$runs/normal.out"
grep -Fq 'Model checking completed. No error has been found.' "$normal" ||
  fail "restored full model did not complete successfully"
if grep -Eq '^Error: Invariant .* is violated\.$' "$normal"; then
  fail "restored full model has an invariant violation"
fi
normal_counts="$(sed -En 's/^([0-9]+) states generated, ([0-9]+) distinct states found, ([0-9]+) states left on queue\.$/\1 \2 \3/p' "$normal")"
[[ "$normal_counts" =~ ^[0-9]+\ [0-9]+\ 0$ ]] ||
  fail "could not parse a complete full baseline state count"
read -r normal_generated normal_distinct normal_left <<<"$normal_counts"
(( normal_distinct >= 100000 && normal_left == 0 )) ||
  fail "unexpected full baseline state result: $normal_counts"

assert_only_full_violation() {
  local output="$1" invariant="$2"
  local count unexpected
  count="$(grep -Fc "Error: Invariant $invariant is violated." "$output" || true)"
  [[ "$count" == "1" ]] ||
    fail "$output has $count expected $invariant violations; expected exactly one"
  unexpected="$(grep -E '^Error: Invariant .* is violated\.$' "$output" | grep -Fvx "Error: Invariant $invariant is violated." || true)"
  [[ -z "$unexpected" ]] || fail "unexpected full-model invariant violation: $unexpected"
}

early="$runs/broken-early-release.out"
assert_only_full_violation "$early" ConfirmedReleaseInsideLease
for expected_value in \
  '<ReleaseConfirmed(r1)' \
  'phase = (r1 :> "expiredPending"' \
  'phase = (r1 :> "completed"' \
  'finishedAt = (r1 :> 3' \
  'deadline = (r1 :> 3'; do
  grep -Fq "$expected_value" "$early" ||
    fail "early-release counterexample is missing: $expected_value"
done

stale="$runs/broken-stale-start.out"
assert_only_full_violation "$stale" ActiveLeaseSafety
for expected_value in \
  '<WorkerStartsUpstream(r1)' \
  'upstreamStart = (r1 :> 2' \
  'deadline = (r1 :> 3' \
  'upstream = (r1 :> "sent"'; do
  grep -Fq "$expected_value" "$stale" ||
    fail "stale-start counterexample is missing: $expected_value"
done

pass_report="$FORMAL/statechart-chart/report.md"
pass_tlc="$FORMAL/statechart-chart/tlc.out"
[[ -s "$FORMAL/statechart-chart/chart.mmd" ]] ||
  fail "normal chart.mmd is missing or empty"
[[ -s "$FORMAL/statechart-broken-early/chart.mmd" ]] ||
  fail "early-release chart.mmd is missing or empty"
[[ -s "$FORMAL/statechart-broken-stale/chart.mmd" ]] ||
  fail "stale-start chart.mmd is missing or empty"
grep -Fqx '## Rules that hold' "$pass_report" ||
  fail "normal chart report has no holding-rule section"
grep -Fq 'TypeOK, LeaseSafety, ConfirmedReleaseInsideLease, SnapshotRecoverySafety, NoDeadlock' "$pass_report" ||
  fail "normal chart did not confirm every rule and deadlock safety"
if grep -Eq '^## .* breaks$|^## Rules not checked' "$pass_report"; then
  fail "normal chart has a broken or unchecked rule"
fi
grep -Fq 'Model checking completed. No error has been found.' "$pass_tlc" ||
  fail "normal chart TLC run did not complete"

assert_chart_failure() {
  local report="$1" tlc="$2" invariant="$3" holding="$4"
  local count
  grep -Fqx "## $invariant breaks" "$report" ||
    fail "chart report did not name the expected $invariant break"
  count="$(grep -Ec '^## .* breaks$' "$report" || true)"
  [[ "$count" == "1" ]] || fail "chart has $count broken rules; expected exactly one"
  grep -Fq '## Rules not checked' "$report" &&
    fail "chart has an unchecked rule" || true
  grep -Fq "$holding" "$report" ||
    fail "chart did not confirm the remaining rules hold: $holding"
  count="$(grep -Fc "Error: Invariant $invariant is violated." "$tlc" || true)"
  [[ "$count" == "1" ]] || fail "chart TLC has $count $invariant violations; expected one"
}

assert_chart_failure \
  "$FORMAL/statechart-broken-early/report.md" \
  "$FORMAL/statechart-broken-early/tlc.out" \
  ConfirmedReleaseInsideLease \
  'TypeOK, LeaseSafety, SnapshotRecoverySafety, NoDeadlock'
grep -Fq 'finishedAt = 3' "$FORMAL/statechart-broken-early/tlc.out" ||
  fail "early-release chart counterexample lacks finishedAt=3"
grep -Fq 'deadline = 3' "$FORMAL/statechart-broken-early/tlc.out" ||
  fail "early-release chart counterexample lacks deadline=3"

assert_chart_failure \
  "$FORMAL/statechart-broken-stale/report.md" \
  "$FORMAL/statechart-broken-stale/tlc.out" \
  LeaseSafety \
  'TypeOK, ConfirmedReleaseInsideLease, SnapshotRecoverySafety, NoDeadlock'
grep -Fq 'upstreamStart = 2' "$FORMAL/statechart-broken-stale/tlc.out" ||
  fail "stale-start chart counterexample lacks upstreamStart=2"
grep -Fq 'deadline = 3' "$FORMAL/statechart-broken-stale/tlc.out" ||
  fail "stale-start chart counterexample lacks deadline=3"

pass_chart_states="$(sed -En 's/^TLC reached ([0-9]+) states.*/\1/p' "$pass_report")"
early_chart_states="$(sed -En 's/^TLC reached ([0-9]+) states.*/\1/p' "$FORMAL/statechart-broken-early/report.md")"
stale_chart_states="$(sed -En 's/^TLC reached ([0-9]+) states.*/\1/p' "$FORMAL/statechart-broken-stale/report.md")"
[[ "$pass_chart_states" =~ ^[0-9]+$ ]] &&
  [[ "$early_chart_states" =~ ^[0-9]+$ ]] &&
  [[ "$stale_chart_states" =~ ^[0-9]+$ ]] ||
  fail "could not parse all chart state counts"

finished_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
model_sha="$(shasum -a 256 "$FORMAL/ExecutionBudgetLeases.tla" | awk '{print $1}')"
runs_java_line="$(head -1 "$runs/java.txt")"
normal_chart="$(cat "$FORMAL/statechart-chart/chart.mmd")"
cat >"$FORMAL/EVIDENCE.md" <<EOF
# F04 execution-budget lease formal verification evidence

## Result

- Property: a cross-replica active lease is unexpired, uniquely bound to one request, counted against the shared quota, and permits only a pre-fence upstream attempt; uncertain expiry remains reserved until reclaim grace. Violating this can over-admit business calls or let a crashed/stale Worker invoke upstream after its permit expires.
- Started / finished (UTC): \`$started_at\` / \`$finished_at\`.
- Tool: TLA+ Tools v1.8.0, TLC build \`2026.11.06.222357 (rev: d8fb231)\`, jar SHA-256 \`$TLA_SHA256\`; Java output: \`$runs_java_line\`.
- Full restored model: \`ExecutionBudgetLeases.cfg\` — $normal_generated generated / $normal_distinct distinct states, $normal_left left; \`TypeOK\`, \`ActiveLeaseSafety\`, \`ConfirmedReleaseInsideLease\`, and \`SnapshotRecoverySafety\` all hold.
- Full early-release mutation: only \`ConfirmedReleaseInsideLease\` is violated. At \`now=3\`, \`r1\` changes from \`expiredPending\` to \`completed\` with \`finishedAt=3\` and \`deadline=3\`, releasing \`reservedCount\` before grace.
- Full stale-start mutation: only \`ActiveLeaseSafety\` is violated. At \`now=2\`, \`r1\` starts upstream for \`deadline=3\`; with fencing margin 1, \`upstreamStart + FencingMargin < deadline\` is false.
- Normal chart: $pass_chart_states states; \`TypeOK\`, \`LeaseSafety\`, \`ConfirmedReleaseInsideLease\`, \`SnapshotRecoverySafety\`, and \`NoDeadlock\` hold; no rule is unchecked.
- Early chart: $early_chart_states states; exactly \`ConfirmedReleaseInsideLease\` breaks, \`TypeOK\`, \`LeaseSafety\`, \`SnapshotRecoverySafety\`, and \`NoDeadlock\` hold, and no rule is unchecked.
- Stale chart: $stale_chart_states states; exactly \`LeaseSafety\` breaks, \`TypeOK\`, \`ConfirmedReleaseInsideLease\`, \`SnapshotRecoverySafety\`, and \`NoDeadlock\` hold, and no rule is unchecked.
- Full model SHA-256: \`$model_sha\`.

## Commands

The repeat command is:

\`\`\`bash
rtk bash .codex/skills/verify-openapi-execution-budgets/scripts/verify.sh
\`\`\`

The script downloads TLA+ Tools only when \`TLA2TOOLS\` is absent, verifies the pinned SHA-256, runs the full model and both full mutations with Java TLC, and runs the normal plus two broken chart models with \`/statechart\`. It individually asserts all three non-empty \`chart.mmd\` files, parses exact invariant names, concrete lease values, absence of unexpected or unchecked rules, and rewrites this evidence file.

Raw full-model outputs are in \`docs/plans/openapi-workers/formal/runs/latest/\`. Chart artifacts are \`statechart-chart/chart.mmd\`, \`statechart-broken-early/chart.mmd\`, and \`statechart-broken-stale/chart.mmd\`, each beside \`report.md\` and \`tlc.out\`.

## Normal Mermaid chart

\`\`\`mermaid
$normal_chart
\`\`\`

The chart hides repeated \`Tick\` and \`Done\` self-steps and interleavings that leave the same visible phase. Those remain in the TLC state graph. The two shortest broken-path charts are \`statechart-broken-early/chart.mmd\` and \`statechart-broken-stale/chart.mmd\`.

## Mapping and boundaries

- Full \`phase\` maps to \`openapi_execution_requests.state\`; \`deadline\` maps to the UTC lease deadline; \`reservedCount\` maps to active/unreclaimed shared quota; \`origin\` maps to \`obot_instance_id\`; \`delivery\` models Obot→Worker dispatch; \`upstreamStart\` models the only permitted business attempt.
- \`Tick\` atomically observes due waiting/active deadlines. \`CancelWaitingAction\` immediately cancels an unexpired waiting ticket without changing active quota. \`SnapshotRequiredAction\` keeps the active lease while awaiting the same-instance immutable snapshot, and \`WorkerStartsUpstream\` is blocked until \`SnapshotRecoveredAction\`. \`ReclaimExpiredAction\` releases quota only after \`deadline + ReclaimGrace\`; \`ObotCrash\` removes an undelivered dispatch so it cannot later start upstream; \`WorkerStartsUpstream\` requires the full fencing margin.
- The scalar chart models are projections for readable Mermaid output because the full function-heavy module is beyond the current statechart XML importer. They do not relax or replace the full baseline invariants.
- Not covered: PostgreSQL transactions/locks, release-token hash comparison, queue fairness among operations, HTTP cancellation propagation, retry/network behavior, payload limits, audit, clock drift beyond the modeled margin, and real deployment. A pass is not implementation, E2E, or deployment evidence.
EOF

echo "PASS: full lease model, $normal_distinct distinct states."
echo "FAIL-AS-EXPECTED: full early release and stale start each trigger exactly one intended invariant."
echo "PASS/FAIL-AS-EXPECTED: charts completed with no unchecked rules."
echo "Evidence: $FORMAL/EVIDENCE.md"
