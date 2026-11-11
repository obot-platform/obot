------------- MODULE OpenAPIBudgetLeasesChartBrokenEarlyRelease -------------

EXTENDS Naturals

CONSTANTS LeaseTTL, ReclaimGrace, FencingMargin, Horizon

VARIABLES phase, now, deadline, upstreamStart, finishedAt, recovery

vars == <<phase, now, deadline, upstreamStart, finishedAt, recovery>>

Init == /\ phase = "idle"
        /\ now = 0
        /\ deadline = 0
        /\ upstreamStart = 0
        /\ finishedAt = 0
        /\ recovery = "not_required"

Tick == /\ now < Horizon
        /\ now' = now + 1
        /\ phase' = IF phase \in {"activeNotSent", "activeSent"} /\ deadline <= now + 1
                      THEN "expiredPending"
                      ELSE phase
        /\ deadline' = deadline
        /\ upstreamStart' = upstreamStart
        /\ finishedAt' = finishedAt
        /\ recovery' = recovery

Enqueue == /\ phase = "idle"
           /\ phase' = "waiting"
           /\ now' = now
           /\ deadline' = deadline
           /\ upstreamStart' = upstreamStart
           /\ finishedAt' = finishedAt
           /\ recovery' = "not_required"

Admit == /\ phase = "waiting"
         /\ now + LeaseTTL <= Horizon
         /\ phase' = "activeNotSent"
         /\ deadline' = now + LeaseTTL
         /\ now' = now
         /\ upstreamStart' = upstreamStart
         /\ finishedAt' = finishedAt
         /\ recovery' = "not_required"

CancelWaiting == /\ phase = "waiting"
                 /\ phase' = "cancelled"
                 /\ finishedAt' = now
                 /\ now' = now
                 /\ deadline' = deadline
                 /\ upstreamStart' = upstreamStart
                 /\ recovery' = "not_required"

SnapshotRequired == /\ phase = "activeNotSent"
                    /\ recovery = "not_required"
                    /\ recovery' = "required"
                    /\ phase' = phase
                    /\ now' = now
                    /\ deadline' = deadline
                    /\ upstreamStart' = upstreamStart
                    /\ finishedAt' = finishedAt

SnapshotRecovered == /\ phase = "activeNotSent"
                     /\ recovery = "required"
                     /\ recovery' = "recovered"
                     /\ phase' = phase
                     /\ now' = now
                     /\ deadline' = deadline
                     /\ upstreamStart' = upstreamStart
                     /\ finishedAt' = finishedAt

WorkerStartsUpstream == /\ phase = "activeNotSent"
                        /\ recovery # "required"
                        /\ now + FencingMargin < deadline
                        /\ phase' = "activeSent"
                        /\ upstreamStart' = now
                        /\ now' = now
                        /\ deadline' = deadline
                        /\ finishedAt' = finishedAt
                        /\ recovery' = recovery

ReleaseConfirmed == \/ /\ phase = "activeSent"
                       /\ now < deadline
                    \/ /\ phase = "expiredPending"
                       /\ upstreamStart < deadline
                    /\ phase' = "completed"
                    /\ finishedAt' = now
                    /\ now' = now
                    /\ deadline' = deadline
                    /\ upstreamStart' = upstreamStart
                    /\ recovery' = recovery

ReclaimExpired == /\ phase = "expiredPending"
                  /\ now >= deadline + ReclaimGrace
                  /\ phase' = "expired"
                  /\ now' = now
                  /\ deadline' = deadline
                  /\ upstreamStart' = upstreamStart
                  /\ finishedAt' = finishedAt
                  /\ recovery' = recovery

Done == /\ now >= Horizon
        /\ UNCHANGED vars

Next == \/ Tick
        \/ Enqueue
        \/ Admit
        \/ CancelWaiting
        \/ SnapshotRequired
        \/ SnapshotRecovered
        \/ WorkerStartsUpstream
        \/ ReleaseConfirmed
        \/ ReclaimExpired
        \/ Done

TypeOK == /\ now \in 0..Horizon
          /\ phase \in {"idle", "waiting", "activeNotSent", "activeSent",
                        "expiredPending", "completed", "cancelled", "expired"}
          /\ deadline \in 0..Horizon
          /\ upstreamStart \in 0..Horizon
          /\ finishedAt \in 0..Horizon
          /\ recovery \in {"not_required", "required", "recovered"}

LeaseSafety == /\ phase \in {"activeNotSent", "activeSent"} => now < deadline
               /\ phase = "activeSent" =>
                    upstreamStart + FencingMargin < deadline

ConfirmedReleaseInsideLease == phase = "completed" => finishedAt < deadline

SnapshotRecoverySafety == recovery = "required" => phase # "activeSent"

=============================================================================
