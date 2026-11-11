--------------------------- MODULE SnapshotFencing ---------------------------
EXTENDS Naturals

CONSTANTS
    EnforceFence,
    MaxRejections

ReqIds == {"r1"}
Digests == {"digest-a", "digest-b"}
NoDigest == "not-bound"
DigestTokens == Digests \cup {NoDigest}

Phases == {
    "new",
    "cache-miss",
    "snapshot-required",
    "compiling",
    "ready",
    "executing",
    "business-done",
    "transport-unknown",
    "snapshot-invalid",
    "compile-failed"
}

SnapshotOutcomes == {"valid", "invalid"}
CompileOutcomes == {"success", "failure"}
TerminalPhases == {
    "business-done",
    "transport-unknown",
    "snapshot-invalid",
    "compile-failed"
}

RequestShape == [
    phase: Phases,
    digest: DigestTokens,
    supplies: 0..1,
    entries: 0..2,
    upstreamAttempts: 0..1,
    reuseRejections: 0..MaxRejections,
    digestMismatchRejections: 0..MaxRejections
]

InitialRequest == [
    phase |-> "new",
    digest |-> NoDigest,
    supplies |-> 0,
    entries |-> 0,
    upstreamAttempts |-> 0,
    reuseRejections |-> 0,
    digestMismatchRejections |-> 0
]

VARIABLES request

vars == <<request>>

TypeOK ==
    /\ EnforceFence \in {TRUE, FALSE}
    /\ request \in [ReqIds -> RequestShape]

SetRequest(r, phase, digest, supplies, entries, upstreamAttempts,
           reuseRejections, digestMismatchRejections) ==
    [request EXCEPT ![r] = [
        phase |-> phase,
        digest |-> digest,
        supplies |-> supplies,
        entries |-> entries,
        upstreamAttempts |-> upstreamAttempts,
        reuseRejections |-> reuseRejections,
        digestMismatchRejections |-> digestMismatchRejections
    ]]

Init == request = [r \in ReqIds |-> InitialRequest]

StartExecute(r, digest) ==
    /\ request[r].phase = "new"
    /\ request[r].digest = NoDigest
    /\ digest \in Digests
    /\ request' = SetRequest(r, "cache-miss", digest,
                             request[r].supplies, request[r].entries,
                             request[r].upstreamAttempts,
                             request[r].reuseRejections,
                             request[r].digestMismatchRejections)

ReturnSnapshotRequired(r) ==
    /\ request[r].phase = "cache-miss"
    /\ request' = SetRequest(r, "snapshot-required", request[r].digest,
                             request[r].supplies, request[r].entries,
                             request[r].upstreamAttempts,
                             request[r].reuseRejections,
                             request[r].digestMismatchRejections)

SupplySnapshot(r, outcome) ==
    /\ request[r].phase = "snapshot-required"
    /\ request[r].supplies = 0
    /\ outcome \in SnapshotOutcomes
    /\ request' = SetRequest(
         r,
         IF outcome = "valid" THEN "compiling" ELSE "snapshot-invalid",
         request[r].digest,
         1,
         request[r].entries,
         request[r].upstreamAttempts,
         request[r].reuseRejections,
         request[r].digestMismatchRejections)

CompileCompletes(r, outcome) ==
    /\ request[r].phase = "compiling"
    /\ outcome \in CompileOutcomes
    /\ request' = SetRequest(
         r,
         IF outcome = "success" THEN "ready" ELSE "compile-failed",
         request[r].digest,
         request[r].supplies,
         request[r].entries,
         request[r].upstreamAttempts,
         request[r].reuseRejections,
         request[r].digestMismatchRejections)

FenceEntryAllowed(r) ==
    request[r].entries = 0 \/ EnforceFence = FALSE

AcquireExecutionFence(r) ==
    /\ request[r].phase \in {"ready", "business-done", "transport-unknown"}
    /\ request[r].entries < 2
    /\ FenceEntryAllowed(r)
    /\ request' = SetRequest(r, "executing", request[r].digest,
                             request[r].supplies,
                             request[r].entries + 1,
                             request[r].upstreamAttempts,
                             request[r].reuseRejections,
                             request[r].digestMismatchRejections)

BeginUpstream(r) ==
    /\ request[r].phase = "executing"
    /\ request[r].upstreamAttempts = 0
    /\ request' = SetRequest(r, "executing", request[r].digest,
                             request[r].supplies, request[r].entries, 1,
                             request[r].reuseRejections,
                             request[r].digestMismatchRejections)

UpstreamCompletes(r) ==
    /\ request[r].phase = "executing"
    /\ request[r].upstreamAttempts = 1
    /\ request' = SetRequest(r, "business-done", request[r].digest,
                             request[r].supplies, request[r].entries,
                             request[r].upstreamAttempts,
                             request[r].reuseRejections,
                             request[r].digestMismatchRejections)

UpstreamTransportFails(r) ==
    /\ request[r].phase = "executing"
    /\ request[r].upstreamAttempts = 1
    /\ request' = SetRequest(r, "transport-unknown", request[r].digest,
                             request[r].supplies, request[r].entries,
                             request[r].upstreamAttempts,
                             request[r].reuseRejections,
                             request[r].digestMismatchRejections)

RejectReuseWithoutFence(r) ==
    /\ request[r].phase \in {"business-done", "transport-unknown"}
    /\ request[r].reuseRejections < MaxRejections
    /\ request' = SetRequest(r, request[r].phase, request[r].digest,
                             request[r].supplies, request[r].entries,
                             request[r].upstreamAttempts,
                             request[r].reuseRejections + 1,
                             request[r].digestMismatchRejections)

RejectDigestMismatch(r, digest) ==
    /\ request[r].phase \in {"business-done", "transport-unknown"}
    /\ digest # request[r].digest
    /\ request[r].digestMismatchRejections < MaxRejections
    /\ request' = SetRequest(r, request[r].phase, request[r].digest,
                             request[r].supplies, request[r].entries,
                             request[r].upstreamAttempts,
                             request[r].reuseRejections,
                             request[r].digestMismatchRejections + 1)

Done ==
    \E r \in ReqIds :
        request[r].phase \in TerminalPhases
        /\ UNCHANGED vars

Next ==
    \/ \E r \in ReqIds, digest \in Digests :
           StartExecute(r, digest)
           \/ AcquireExecutionFence(r)
           \/ BeginUpstream(r)
           \/ UpstreamCompletes(r)
           \/ UpstreamTransportFails(r)
           \/ RejectDigestMismatch(r, digest)
    \/ \E r \in ReqIds :
           ReturnSnapshotRequired(r)
           \/ SupplySnapshot(r, "valid")
           \/ SupplySnapshot(r, "invalid")
           \/ CompileCompletes(r, "success")
           \/ CompileCompletes(r, "failure")
           \/ RejectReuseWithoutFence(r)
    \/ Done

SnapshotSuppliedAtMostOnce ==
    \A r \in ReqIds : request[r].supplies <= 1

ExecutionEnteredAtMostOnce ==
    \A r \in ReqIds : request[r].entries <= 1

UpstreamAttemptedAtMostOnce ==
    \A r \in ReqIds : request[r].upstreamAttempts <= 1

UpstreamRequiresOneSnapshotAndOneFence ==
    \A r \in ReqIds :
        request[r].upstreamAttempts >= 1 =>
            /\ request[r].supplies = 1
            /\ request[r].entries = 1
            /\ request[r].phase \in {
                   "executing",
                   "business-done",
                   "transport-unknown"
               }

SnapshotRequiredNeverTouchesUpstream ==
    \A r \in ReqIds :
        request[r].phase \in {
            "new",
            "cache-miss",
            "snapshot-required",
            "compiling",
            "ready"
        } =>
            request[r].upstreamAttempts = 0

NoRetryAfterUncertainOutcome ==
    \A r \in ReqIds :
        request[r].phase = "transport-unknown" =>
            request[r].entries = 1 /\ request[r].upstreamAttempts = 1

=============================================================================
