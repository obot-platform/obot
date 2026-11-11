#!/usr/bin/env bash
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
MODEL_DIR="$ROOT/docs/plans/openapi-workers/formal"
OUT_DIR="${1:-$MODEL_DIR/runs/latest}"
JAR_URL="https://github.com/tlaplus/tlaplus/releases/download/v1.8.0/tla2tools.jar"
JAR_SHA256="e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f"
EXPECTED_NORMAL="Model checking completed. No error has been found."
EXPECTED_BROKEN="Invariant ExecutionRequiresUniqueAuthorizedMatch is violated."
STATECHART_TS="${STATECHART_TS:-$HOME/.agents/skills/statechart/scripts/statechart.ts}"
STATECHART_BIN="${STATECHART_BIN:-bun}"

mkdir -p "$OUT_DIR"
rm -f "$OUT_DIR"/*.out "$OUT_DIR"/*.txt "$OUT_DIR"/*.md
rm -rf "$OUT_DIR/statechart-normal" "$OUT_DIR/statechart-broken"
rm -f "$MODEL_DIR"/*_TTrace_*.tla "$MODEL_DIR"/*_TTrace_*.bin
rm -rf "$MODEL_DIR/states"
STARTED_AT="$(date +%Y-%m-%dT%H:%M:%S%z)"

TMP_DIR="$(mktemp -d)"
cleanup_debug_outputs() {
  rm -f "$MODEL_DIR"/*_TTrace_*.tla "$MODEL_DIR"/*_TTrace_*.bin
  rm -rf "$MODEL_DIR/states"
}
trap 'cleanup_debug_outputs; rm -rf "$TMP_DIR"' EXIT
JAR_PATH="${TLA2_TOOLS_JAR:-$TMP_DIR/tla2tools.jar}"
if [[ ! -f "$JAR_PATH" ]]; then
  curl --fail --location --silent --show-error "$JAR_URL" --output "$JAR_PATH" || {
    echo "failed to download pinned tla2tools.jar" >&2
    exit 2
  }
fi
ACTUAL_SHA="$(shasum -a 256 "$JAR_PATH" | awk '{print $1}')"
if [[ "$ACTUAL_SHA" != "$JAR_SHA256" ]]; then
  echo "tla2tools.jar SHA-256 mismatch: $ACTUAL_SHA" >&2
  exit 2
fi

JAVA_BIN="${JAVA:-java}"
"$JAVA_BIN" -version >"$OUT_DIR/java.txt" 2>&1 || exit 2
"$STATECHART_BIN" --version >"$OUT_DIR/statechart-tool.txt" 2>&1 || exit 2
[[ -f "$STATECHART_TS" ]] || {
  echo "statechart entry not found: $STATECHART_TS" >&2
  exit 2
}

for invariant in \
  ExecutionRequiresUniqueAuthorizedMatch \
  RawRequestSemanticsPreservedOnExecution \
  RejectionNeverCallsUpstream \
  OnlyAuthorizedExecutionsAreRetained
do
  grep -E "^[[:space:]]*${invariant}[[:space:]]*$" "$MODEL_DIR/OpenAPIHTTPProxy.cfg" >/dev/null || {
    echo "normal config omits invariant $invariant" >&2
    exit 2
  }
done

set +e
(cd "$MODEL_DIR" && "$JAVA_BIN" -XX:+UseParallelGC -cp "$JAR_PATH" tlc2.TLC -workers 1 -cleanup \
  -config OpenAPIHTTPProxy.cfg OpenAPIHTTPProxy.tla) >"$OUT_DIR/normal.out" 2>&1
NORMAL_RC=$?
(cd "$MODEL_DIR" && "$JAVA_BIN" -XX:+UseParallelGC -cp "$JAR_PATH" tlc2.TLC -workers 1 -cleanup \
  -config OpenAPIHTTPProxyBroken.cfg OpenAPIHTTPProxyBroken.tla) >"$OUT_DIR/broken.out" 2>&1
BROKEN_RC=$?
set -e

TLA2TOOLS="$JAR_PATH" JAVA="$JAVA_BIN" "$STATECHART_BIN" "$STATECHART_TS" \
  "$MODEL_DIR/OpenAPIHTTPProxyChart.tla" \
  --config "$MODEL_DIR/OpenAPIHTTPProxyChart.cfg" \
  --view "$MODEL_DIR/statechart.mjs" \
  --out "$OUT_DIR/statechart-normal" >"$OUT_DIR/statechart-normal.console.out" 2>&1
CHART_NORMAL_RC=$?

set +e
TLA2TOOLS="$JAR_PATH" JAVA="$JAVA_BIN" "$STATECHART_BIN" "$STATECHART_TS" \
  "$MODEL_DIR/OpenAPIHTTPProxyChartBroken.tla" \
  --config "$MODEL_DIR/OpenAPIHTTPProxyChartBroken.cfg" \
  --view "$MODEL_DIR/statechart.mjs" \
  --out "$OUT_DIR/statechart-broken" >"$OUT_DIR/statechart-broken.console.out" 2>&1
CHART_BROKEN_RC=$?
set -e
FINISHED_AT="$(date +%Y-%m-%dT%H:%M:%S%z)"

NORMAL_STATES="$(grep -E '^[0-9]+ states generated, [0-9]+ distinct states found, 0 states left on queue\.$' "$OUT_DIR/normal.out" | tail -1)"
BROKEN_STATES="$(grep -E '^[0-9]+ states generated, [0-9]+ distinct states found, [0-9]+ states left on queue\.$' "$OUT_DIR/broken.out" | tail -1)"

fail() {
  echo "$1" >&2
  echo "See $OUT_DIR" >&2
  exit 2
}

[[ "$NORMAL_RC" -eq 0 ]] || fail "normal TLC exited $NORMAL_RC; expected 0"
grep -F "$EXPECTED_NORMAL" "$OUT_DIR/normal.out" >/dev/null || fail "normal TLC did not report all rules passed"
if grep -E '^(Error:|Warning:)|is violated\.|Errors: [1-9]' "$OUT_DIR/normal.out" >/dev/null; then
  fail "normal TLC contains unexpected error, warning, or violation"
fi
[[ -n "$NORMAL_STATES" ]] || fail "normal TLC state summary missing"

[[ "$BROKEN_RC" -eq 12 ]] || fail "full broken TLC exited $BROKEN_RC; expected TLA+ 1.8.0 invariant exit 12"
VIOLATIONS="$(grep -c '^Error: Invariant .* is violated\.$' "$OUT_DIR/broken.out")"
[[ "$VIOLATIONS" -eq 1 ]] || fail "broken TLC reported $VIOLATIONS invariant violations; expected exactly 1"
grep -F "$EXPECTED_BROKEN" "$OUT_DIR/broken.out" >/dev/null || fail "broken TLC violated an unexpected invariant"
if grep -E '^Error:' "$OUT_DIR/broken.out" |
  grep -vF 'Error: Invariant ExecutionRequiresUniqueAuthorizedMatch is violated.' |
  grep -vF 'Error: The behavior up to this point is:' >/dev/null; then
  fail "broken TLC contains an unexpected TLC error"
fi
[[ -n "$BROKEN_STATES" ]] || fail "broken TLC state summary missing"

[[ "$CHART_NORMAL_RC" -eq 0 ]] || fail "normal statechart exited $CHART_NORMAL_RC; expected 0"
[[ "$CHART_BROKEN_RC" -eq 1 ]] || fail "broken statechart exited $CHART_BROKEN_RC; expected two-rule violation exit 1"
CHART_NORMAL_REPORT="$OUT_DIR/statechart-normal/report.md"
CHART_BROKEN_REPORT="$OUT_DIR/statechart-broken/report.md"
CHART_NORMAL_CHART="$OUT_DIR/statechart-normal/chart.mmd"
CHART_BROKEN_CHART="$OUT_DIR/statechart-broken/chart.mmd"
for required in "$CHART_NORMAL_REPORT" "$CHART_NORMAL_CHART" "$CHART_BROKEN_REPORT" "$CHART_BROKEN_CHART"; do
  [[ -s "$required" ]] || fail "statechart did not write $required"
done
for holding_rule in \
  ExecutionRequiresUniqueRoute \
  ExecutionRequiresWhitelist \
  RawRequestSemanticsPreservedOnExecution \
  RejectionNeverCallsUpstream \
  NoDeadlock
do
  grep -F "$holding_rule" "$CHART_NORMAL_REPORT" >/dev/null || fail "normal statechart omits held rule $holding_rule"
done
if grep -F "breaks" "$CHART_NORMAL_REPORT" >/dev/null; then
  fail "normal statechart unexpectedly reports a broken rule"
fi
for state_label in \
  "Raw path and query parsed" \
  "One operation uniquely matched" \
  "Operation whitelist passed" \
  "Business request executed" \
  "Rejected: no unique route" \
  "Rejected: operation disabled"
do
  grep -F "$state_label" "$CHART_NORMAL_CHART" >/dev/null || fail "normal statechart omits state $state_label"
done
for broken_rule in ExecutionRequiresUniqueRoute ExecutionRequiresWhitelist; do
  grep -F "## ${broken_rule} breaks" "$CHART_BROKEN_REPORT" >/dev/null || fail "broken statechart omits expected counterexample $broken_rule"
done
if grep -F "## RawRequestSemanticsPreservedOnExecution breaks" "$CHART_BROKEN_REPORT" >/dev/null ||
   grep -F "## RejectionNeverCallsUpstream breaks" "$CHART_BROKEN_REPORT" >/dev/null; then
  fail "broken statechart contains an unexpected counterexample"
fi
for broken_arrow in \
  "the broken variant executes one of the ambiguous routes" \
  "the broken variant skips the whitelist"
do
  grep -F "$broken_arrow" "$CHART_BROKEN_CHART" >/dev/null || fail "broken statechart omits arrow $broken_arrow"
done
CHART_NORMAL_SUMMARY="$(grep -E '^TLC reached [0-9]+ states and [0-9]+ steps\.' "$CHART_NORMAL_REPORT" | tail -1)"
CHART_BROKEN_SUMMARY="$(grep -E '^TLC reached [0-9]+ states and [0-9]+ steps\.' "$CHART_BROKEN_REPORT" | tail -1)"
[[ -n "$CHART_NORMAL_SUMMARY" ]] || fail "normal statechart state summary missing"
[[ -n "$CHART_BROKEN_SUMMARY" ]] || fail "broken statechart state summary missing"

cat >"$OUT_DIR/EVIDENCE.md" <<EVIDENCE
# P03 HTTP proxy formal verification evidence

- Started / finished: \`${STARTED_AT}\` / \`${FINISHED_AT}\`
- Command: \`rtk bash .codex/skills/verify-openapi-http-proxy/scripts/verify.sh\`
- TLA+ Tools release: v1.8.0, SHA-256 \`${JAR_SHA256}\`
- Java:

\`\`\`text
$(cat "$OUT_DIR/java.txt")
\`\`\`
- Statechart runner: \`${STATECHART_BIN} ${STATECHART_TS}\`
- Statechart tool:

\`\`\`text
$(cat "$OUT_DIR/statechart-tool.txt")
\`\`\`

- Normal model: PASS — all four invariants and deadlock check completed with no errors.
- Broken variant: expected failure — only \`ExecutionRequiresUniqueAuthorizedMatch\` was violated.
- Normal states: \`${NORMAL_STATES}\`
- Broken states before stop: \`${BROKEN_STATES}\`
- Normal chart projection: PASS — \`${CHART_NORMAL_SUMMARY}\`; report/chart under \`statechart-normal/\`.
- Broken chart projection: expected failures — unique-route fallthrough and whitelist bypass; \`${CHART_BROKEN_SUMMARY}\`; report/chart under \`statechart-broken/\`.
- Full model uses escaped segment sequences and exact static-precedence matching. The chart model uses the same finite route shapes after strict raw parsing and omits standard-module imports required only by the chart runner; this projection does not weaken the full model invariants.
- Full TLC output: \`normal.out\`, \`broken.out\`.

The verifier checks the pinned artifact hash, TLC exit codes, exact pass marker, exact expected Broken invariant, absence of unexpected invariant/error output, statechart key states/arrows, both expected chart counterexamples, and final state summaries. It does not treat a bare process exit code as sufficient evidence.
EVIDENCE

printf '%s\n' "PASS normal: $NORMAL_STATES"
printf '%s\n' "PASS broken-trigger: $EXPECTED_BROKEN ($BROKEN_STATES)"
printf '%s\n' "PASS normal chart: $CHART_NORMAL_SUMMARY"
printf '%s\n' "PASS broken chart: two expected counterexamples ($CHART_BROKEN_SUMMARY)"
printf '%s\n' "Evidence: $OUT_DIR/EVIDENCE.md"
