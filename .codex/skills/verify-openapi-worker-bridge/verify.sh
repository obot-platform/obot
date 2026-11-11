#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
FORMAL="$ROOT/docs/plans/openapi-workers/formal"
STATECHART="${STATECHART:-$HOME/.agents/skills/statechart/scripts/statechart}"
TLC_URL="https://github.com/tlaplus/tlaplus/releases/download/v1.8.0/tla2tools.jar"
TLC_SHA256="e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f"
SCRATCH_JAR=""

cleanup() {
  if [[ -n "$SCRATCH_JAR" && -f "$SCRATCH_JAR" ]]; then
    rm -f "$SCRATCH_JAR"
  fi
}
trap cleanup EXIT

if [[ ! -x "$STATECHART" ]]; then
  echo "statechart executable not found: $STATECHART" >&2
  exit 2
fi

if [[ -z "${TLA2TOOLS:-}" ]]; then
  SCRATCH_JAR="$(mktemp /tmp/verify-openapi-worker-bridge-tla2tools-1.8.0.XXXXXX.jar)"
  curl -fsSL "$TLC_URL" -o "$SCRATCH_JAR"
  echo "$TLC_SHA256  $SCRATCH_JAR" | shasum -a 256 -c - >/dev/null
  export TLA2TOOLS="$SCRATCH_JAR"
fi

started_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
rm -rf "$FORMAL/statechart-pass" "$FORMAL/statechart-broken"
rm -f "$FORMAL/statechart-pass.console.txt" "$FORMAL/statechart-broken.console.txt"

set +e
"$STATECHART" "$FORMAL/OpenAPIWorkerResolver.tla" \
  --config "$FORMAL/OpenAPIWorkerResolver.cfg" \
  --view "$FORMAL/view.mjs" \
  --out "$FORMAL/statechart-pass" \
  >"$FORMAL/statechart-pass.console.txt" 2>&1
pass_status=$?
"$STATECHART" "$FORMAL/OpenAPIWorkerResolver.tla" \
  --config "$FORMAL/OpenAPIWorkerResolverBroken.cfg" \
  --view "$FORMAL/view.mjs" \
  --out "$FORMAL/statechart-broken" \
  >"$FORMAL/statechart-broken.console.txt" 2>&1
broken_status=$?
set -e

if [[ $pass_status -ne 0 ]]; then
  cat "$FORMAL/statechart-pass.console.txt" >&2
  echo "Expected the restored model to pass, but statechart exited $pass_status." >&2
  exit 1
fi
if [[ $broken_status -ne 1 ]]; then
  cat "$FORMAL/statechart-broken.console.txt" >&2
  echo "Expected the broken model to fail with exactly one invariant violation, but statechart exited $broken_status." >&2
  exit 1
fi

pass_report="$FORMAL/statechart-pass/report.md"
pass_tlc="$FORMAL/statechart-pass/tlc.out"
broken_report="$FORMAL/statechart-broken/report.md"
broken_tlc="$FORMAL/statechart-broken/tlc.out"

for required in ExactCallerCredentialOnly TypeOK NoDeadlock CHECK_DEADLOCK; do
  grep -Fqx "ExactCallerCredentialOnly, TypeOK, NoDeadlock, CHECK_DEADLOCK" "$pass_report" ||
    grep -Fq "$required" "$pass_report" ||
    {
      echo "Restored model did not report $required as holding." >&2
      exit 1
    }
done

if grep -Eq '^## .* breaks$|^## Rules not checked' "$pass_report"; then
  echo "Restored model has an unexpected violation or an unchecked rule." >&2
  exit 1
fi

grep -Fqx '## ExactCallerCredentialOnly breaks' "$broken_report" ||
  {
    echo "Broken model did not report the expected ExactCallerCredentialOnly violation." >&2
    exit 1
  }
break_count="$(grep -Ec '^## .* breaks$' "$broken_report" || true)"
if [[ "$break_count" != "1" ]]; then
  echo "Broken model reported $break_count invariant violations; expected exactly one." >&2
  exit 1
fi
if grep -Fq '## Rules not checked' "$broken_report"; then
  echo "Broken model has an unchecked rule." >&2
  exit 1
fi
for required in TypeOK NoDeadlock CHECK_DEADLOCK; do
  grep -Fq "$required" "$broken_report" ||
    {
      echo "Broken model did not confirm $required still holds." >&2
      exit 1
    }
done

grep -Fq 'TLC2 Version 2026.11.06.222357 (rev: d8fb231)' "$pass_tlc" ||
  {
    echo "Pass run did not use TLA+ Tools v1.8.0 / TLC build 2026.11.06.222357." >&2
    exit 1
  }
grep -Fq 'Model checking completed. No error has been found.' "$pass_tlc" ||
  {
    echo "Pass run did not complete TLC successfully." >&2
    exit 1
  }

violation_count="$(grep -Fc 'Error: Invariant ExactCallerCredentialOnly is violated.' "$broken_tlc" || true)"
if [[ "$violation_count" != "1" ]]; then
  echo "Broken TLC output has $violation_count expected invariant violations; expected exactly one." >&2
  exit 1
fi
unexpected_invariants="$(grep -E '^Error: Invariant .* is violated\.$' "$broken_tlc" | grep -Fvx 'Error: Invariant ExactCallerCredentialOnly is violated.' || true)"
if [[ -n "$unexpected_invariants" ]]; then
  echo "Broken model has an unexpected invariant violation:" >&2
  echo "$unexpected_invariants" >&2
  exit 1
fi
for expected_value in \
  'caller = "u2"' \
  'requestedInstance = "i2"' \
  'credentialSource = "i1"' \
  'owner |-> "u1"'; do
  grep -Fq "$expected_value" "$broken_tlc" ||
    {
      echo "Expected credential-isolation counterexample is missing: $expected_value" >&2
      exit 1
    }
done

pass_states="$(sed -En 's/^TLC reached ([0-9]+) states.*/\1/p' "$pass_report")"
broken_states="$(sed -En 's/^TLC reached ([0-9]+) states.*/\1/p' "$broken_report")"
if [[ ! "$pass_states" =~ ^[0-9]+$ ]] || [[ ! "$broken_states" =~ ^[0-9]+$ ]]; then
  echo "Could not parse state counts from both reports." >&2
  exit 1
fi
finished_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

cat >"$FORMAL/RUN.md" <<EOF
# Formal verification rerun evidence

- Started (UTC): $started_at
- Finished (UTC): $finished_at
- TLA+ Tools: v1.8.0, TLC build 2026.11.06.222357, revision d8fb231, SHA-256 $TLC_SHA256
- Restored model: PASS; ExactCallerCredentialOnly, TypeOK, NoDeadlock, and CHECK_DEADLOCK hold; $pass_states distinct states.
- Broken model: FAIL as expected; only ExactCallerCredentialOnly is violated; TypeOK, NoDeadlock, and CHECK_DEADLOCK hold; $broken_states distinct states.
- Expected counterexample: caller u2 requests i2, but fallback selects i1's credential owned by u1.
- Model boundary: two users, two instances, one component, one tool; transport, retries, token refresh, encryption, and upstream semantics are not modeled.
EOF

echo "PASS: ExactCallerCredentialOnly plus TypeOK/NoDeadlock ($pass_states states)."
echo "FAIL-AS-EXPECTED: only ExactCallerCredentialOnly ($broken_states states)."
echo "Evidence: $FORMAL/RUN.md"
