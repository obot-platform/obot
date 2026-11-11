------------ MODULE OpenAPIBudgetLeasesChartBrokenStaleStart ------------

EXTENDS Naturals

CONSTANTS LeaseTTL, ReclaimGrace, FencingMargin, Horizon

VARIABLES phase, now, deadline, upstreamStart, finishedAt

vars == <<phase, now, deadline, upstreamStart, finishedAt>>

Init == /\ phase = "idle"
        /\ now = 0
        /\ deadline = 0
        /\ upstreamStart = 0
        /\ finishedAt = 0

Tick == /\ now < Horizon
        /\ now' = now + 1
        /\ phase' = IF phase \in {"activeNotSent", "activeSent"} /\ deadline <= now + 1
                      THEN "expiredPending"
                      ELSE phase
        /\ deadline' = deadline
        /\ upstreamStart' = upstreamStart
        /\ finishedAt' = finishedAt

Enqueue == /\ phase = "idle"
           /\ phase' = "waiting"
           /\ now' = now
           /\ deadline' = deadline
           /\ upstreamStart' = upstreamStart
           /\ finishedAt' = finishedAt

Admit == /\ phase = "waiting"
         /\ now + LeaseTTL <= Horizon
         /\ phase' = "activeNotSent"
         /\ deadline' = now + LeaseTTL
         /\ now' = now
         /\ upstreamStart' = upstreamStart
         /\ finishedAt' = finishedAt

WorkerStartsUpstream == /\ phase = "activeNotSent"
                        /\ now + FencingMargin <= deadline
                        /\ phase' = "activeSent"
                        /\ upstreamStart' = now
                        /\ now' = now
                        /\ deadline' = deadline
                        /\ finishedAt' = finishedAt

ReleaseConfirmed == /\ phase = "activeSent"
                    /\ now < deadline
                    /\ phase' = "completed"
                    /\ finishedAt' = now
                    /\ now' = now
                    /\ deadline' = deadline
                    /\ upstreamStart' = upstreamStart

ReclaimExpired == /\ phase = "expiredPending"
                  /\ now >= deadline + ReclaimGrace
                  /\ phase' = "expired"
                  /\ now' = now
                  /\ deadline' = deadline
                  /\ upstreamStart' = upstreamStart
                  /\ finishedAt' = finishedAt

Done == /\ now >= Horizon
        /\ UNCHANGED vars

Next == \/ Tick
        \/ Enqueue
        \/ Admit
        \/ WorkerStartsUpstream
        \/ ReleaseConfirmed
        \/ ReclaimExpired
        \/ Done

TypeOK == /\ now \in 0..Horizon
          /\ phase \in {"idle", "waiting", "activeNotSent", "activeSent",
                        "expiredPending", "completed", "expired"}
          /\ deadline \in 0..Horizon
          /\ upstreamStart \in 0..Horizon
          /\ finishedAt \in 0..Horizon

LeaseSafety == /\ phase \in {"activeNotSent", "activeSent"} => now < deadline
               /\ phase = "activeSent" =>
                    upstreamStart + FencingMargin < deadline

ConfirmedReleaseInsideLease == phase = "completed" => finishedAt < deadline

=============================================================================
