----------------------------- MODULE ExecutionBudgetLeases -----------------------------

EXTENDS Naturals, TLC, FiniteSets

(* This design model covers the cross-replica lease lifecycle proposed in P04 sections 5 *)
(* and 7. It abstracts PostgreSQL rows as functions over a fixed set of request IDs.      *)
(* Time is discrete; a Tick atomically marks due waiting tickets and active leases        *)
(* expired, matching the database transition at which deadline observation becomes state. *)

CONSTANTS
    Requests,
    Replicas,
    MaxActive,
    MaxWaiting,
    LeaseTTL,
    WaitBudget,
    ReclaimGrace,
    FencingMargin,
    Horizon,
    BrokenEarlyRelease,
    BrokenStaleStart

VARIABLES
    now,
    phase,
    origin,
    deadline,
    waitDeadline,
    generation,
    delivery,
    upstream,
    upstreamStart,
    finishedAt,
    recovery,
    reservedCount

vars == <<now, phase, origin, deadline, waitDeadline, generation, delivery,
          upstream, upstreamStart, finishedAt, recovery, reservedCount>>

Phases == {"new", "waiting", "active", "expiredPending", "completed", "cancelled", "expired"}

Init == /\ now = 0
        /\ phase = [r \in Requests |-> "new"]
        /\ origin = [r \in Requests |-> "none"]
        /\ deadline = [r \in Requests |-> 0]
        /\ waitDeadline = [r \in Requests |-> 0]
        /\ generation = [r \in Requests |-> 0]
        /\ delivery = [r \in Requests |-> "none"]
        /\ upstream = [r \in Requests |-> "none"]
        /\ upstreamStart = [r \in Requests |-> 0]
        /\ finishedAt = [r \in Requests |-> 0]
        /\ recovery = [r \in Requests |-> "not_required"]
        /\ reservedCount = 0

WaitingTickets == {r \in Requests : phase[r] = "waiting"}
UnreclaimedLeases == {r \in Requests : phase[r] \in {"active", "expiredPending"}}

Tick == /\ now < Horizon
        /\ now' = now + 1
        /\ phase' = [r \in Requests |->
             IF phase[r] = "active" /\ deadline[r] <= now + 1
                THEN "expiredPending"
                ELSE IF phase[r] = "waiting" /\ waitDeadline[r] <= now + 1
                        THEN "expired"
                        ELSE phase[r]]
        /\ origin' = origin
        /\ deadline' = deadline
        /\ waitDeadline' = waitDeadline
        /\ generation' = generation
        /\ delivery' = delivery
        /\ upstream' = upstream
        /\ upstreamStart' = upstreamStart
        /\ finishedAt' = finishedAt
        /\ recovery' = recovery
        /\ reservedCount' = reservedCount

Enqueue(request, replica) ==
    /\ phase[request] = "new"
    /\ Cardinality(WaitingTickets) < MaxWaiting
    /\ now + WaitBudget + LeaseTTL + ReclaimGrace <= Horizon
    /\ phase' = [phase EXCEPT ![request] = "waiting"]
    /\ origin' = [origin EXCEPT ![request] = replica]
    /\ waitDeadline' = [waitDeadline EXCEPT ![request] = now + WaitBudget]
    /\ deadline' = deadline
    /\ generation' = generation
    /\ delivery' = [delivery EXCEPT ![request] = "none"]
    /\ upstream' = [upstream EXCEPT ![request] = "none"]
    /\ upstreamStart' = upstreamStart
    /\ finishedAt' = finishedAt
        /\ recovery' = [recovery EXCEPT ![request] = "not_required"]
    /\ reservedCount' = reservedCount
    /\ now' = now

Admit(request) ==
    /\ phase[request] = "waiting"
    /\ now < waitDeadline[request]
    /\ reservedCount < MaxActive
    /\ phase' = [phase EXCEPT ![request] = "active"]
    /\ deadline' = [deadline EXCEPT ![request] = now + LeaseTTL]
    /\ generation' = [generation EXCEPT ![request] = generation[request] + 1]
    /\ delivery' = [delivery EXCEPT ![request] = "none"]
    /\ upstream' = [upstream EXCEPT ![request] = "none"]
    /\ upstreamStart' = upstreamStart
    /\ finishedAt' = finishedAt
        /\ recovery' = [recovery EXCEPT ![request] = "not_required"]
    /\ reservedCount' = reservedCount + 1
    /\ origin' = origin
    /\ waitDeadline' = waitDeadline
    /\ now' = now

DispatchToWorker(request) ==
    /\ phase[request] = "active"
    /\ delivery[request] = "none"
    /\ now + FencingMargin < deadline[request]
    /\ delivery' = [delivery EXCEPT ![request] = "delivered"]
    /\ phase' = phase
    /\ origin' = origin
    /\ deadline' = deadline
    /\ waitDeadline' = waitDeadline
    /\ generation' = generation
    /\ upstream' = upstream
    /\ upstreamStart' = upstreamStart
    /\ finishedAt' = finishedAt
        /\ recovery' = [recovery EXCEPT ![request] = "not_required"]
    /\ reservedCount' = reservedCount
    /\ now' = now

CancelWaiting(request) ==
    /\ phase[request] = "waiting"
    /\ now < waitDeadline[request]
    /\ phase' = [phase EXCEPT ![request] = "cancelled"]
    /\ finishedAt' = [finishedAt EXCEPT ![request] = now]
    /\ reservedCount' = reservedCount
    /\ origin' = origin
    /\ deadline' = deadline
    /\ waitDeadline' = waitDeadline
    /\ generation' = generation
    /\ delivery' = [delivery EXCEPT ![request] = "none"]
    /\ upstream' = [upstream EXCEPT ![request] = "none"]
    /\ upstreamStart' = upstreamStart
    /\ recovery' = [recovery EXCEPT ![request] = "not_required"]
    /\ now' = now

ObotCrash(request) ==
    /\ phase[request] = "active"
    /\ delivery[request] = "delivered"
    /\ upstream[request] = "none"
    /\ delivery' = [delivery EXCEPT ![request] = "lost"]
    /\ phase' = phase
    /\ origin' = origin
    /\ deadline' = deadline
    /\ waitDeadline' = waitDeadline
    /\ generation' = generation
    /\ upstream' = upstream
    /\ upstreamStart' = upstreamStart
    /\ finishedAt' = finishedAt
        /\ recovery' = recovery
    /\ reservedCount' = reservedCount
    /\ now' = now

SnapshotRequired(request) ==
    /\ phase[request] = "active"
    /\ delivery[request] = "delivered"
    /\ upstream[request] = "none"
    /\ recovery[request] = "not_required"
    /\ recovery' = [recovery EXCEPT ![request] = "required"]
    /\ phase' = phase
    /\ origin' = origin
    /\ deadline' = deadline
    /\ waitDeadline' = waitDeadline
    /\ generation' = generation
    /\ delivery' = delivery
    /\ upstream' = upstream
    /\ upstreamStart' = upstreamStart
    /\ finishedAt' = finishedAt
    /\ reservedCount' = reservedCount
    /\ now' = now

SnapshotRecovered(request) ==
    /\ phase[request] = "active"
    /\ recovery[request] = "required"
    /\ recovery' = [recovery EXCEPT ![request] = "recovered"]
    /\ phase' = phase
    /\ origin' = origin
    /\ deadline' = deadline
    /\ waitDeadline' = waitDeadline
    /\ generation' = generation
    /\ delivery' = delivery
    /\ upstream' = upstream
    /\ upstreamStart' = upstreamStart
    /\ finishedAt' = finishedAt
    /\ reservedCount' = reservedCount
    /\ now' = now

WorkerStartsUpstream(request) ==
    /\ phase[request] = "active"
    /\ delivery[request] = "delivered"
    /\ upstream[request] = "none"
    /\ recovery[request] # "required"
    /\ (now + FencingMargin < deadline[request] \/ BrokenStaleStart)
    /\ upstream' = [upstream EXCEPT ![request] = "sent"]
    /\ upstreamStart' = [upstreamStart EXCEPT ![request] = now]
    /\ phase' = phase
    /\ origin' = origin
    /\ deadline' = deadline
    /\ waitDeadline' = waitDeadline
    /\ generation' = generation
    /\ delivery' = delivery
    /\ finishedAt' = finishedAt
        /\ recovery' = recovery
    /\ reservedCount' = reservedCount
    /\ now' = now

ReleaseConfirmed(request) ==
    /\ \/ /\ phase[request] = "active"
          /\ upstream[request] = "sent"
          /\ now < deadline[request]
       \/ /\ BrokenEarlyRelease
          /\ phase[request] = "expiredPending"
          /\ upstream[request] = "sent"
    /\ phase' = [phase EXCEPT ![request] = "completed"]
    /\ finishedAt' = [finishedAt EXCEPT ![request] = now]
    /\ reservedCount' = reservedCount - 1
    /\ upstream' = upstream
    /\ upstreamStart' = upstreamStart
    /\ origin' = origin
    /\ deadline' = deadline
    /\ waitDeadline' = waitDeadline
    /\ generation' = generation
    /\ delivery' = delivery
        /\ recovery' = recovery
    /\ now' = now

CancelConfirmed(request) ==
    /\ phase[request] = "active"
    /\ now < deadline[request]
    /\ upstream[request] = "none"
    /\ phase' = [phase EXCEPT ![request] = "cancelled"]
    /\ finishedAt' = [finishedAt EXCEPT ![request] = now]
    /\ reservedCount' = reservedCount - 1
    /\ upstream' = upstream
    /\ upstreamStart' = upstreamStart
    /\ recovery' = recovery
    /\ origin' = origin
    /\ deadline' = deadline
    /\ waitDeadline' = waitDeadline
    /\ generation' = generation
    /\ delivery' = delivery
    /\ now' = now

ReclaimExpired(request) ==
    /\ phase[request] = "expiredPending"
    /\ now >= deadline[request] + ReclaimGrace
    /\ phase' = [phase EXCEPT ![request] = "expired"]
    /\ reservedCount' = reservedCount - 1
    /\ phase' /= phase
    /\ upstream' = upstream
    /\ upstreamStart' = upstreamStart
    /\ origin' = origin
    /\ deadline' = deadline
    /\ waitDeadline' = waitDeadline
    /\ generation' = generation
    /\ delivery' = delivery
        /\ recovery' = recovery
    /\ finishedAt' = finishedAt
    /\ now' = now

Done == /\ now >= Horizon
        /\ UNCHANGED vars

EnqueueAction == \E request \in Requests, replica \in Replicas :
    Enqueue(request, replica)

AdmitAction == \E request \in Requests : Admit(request)
DispatchToWorkerAction == \E request \in Requests : DispatchToWorker(request)
ObotCrashAction == \E request \in Requests : ObotCrash(request)
WorkerStartsUpstreamAction == \E request \in Requests :
    WorkerStartsUpstream(request)
ReleaseConfirmedAction == \E request \in Requests : ReleaseConfirmed(request)
CancelConfirmedAction == \E request \in Requests : CancelConfirmed(request)
CancelWaitingAction == \E request \in Requests : CancelWaiting(request)
SnapshotRequiredAction == \E request \in Requests : SnapshotRequired(request)
SnapshotRecoveredAction == \E request \in Requests : SnapshotRecovered(request)
ReclaimExpiredAction == \E request \in Requests : ReclaimExpired(request)

Next == \/ Tick
        \/ EnqueueAction
        \/ AdmitAction
        \/ DispatchToWorkerAction
        \/ ObotCrashAction
        \/ WorkerStartsUpstreamAction
        \/ ReleaseConfirmedAction
        \/ CancelConfirmedAction
        \/ CancelWaitingAction
        \/ SnapshotRequiredAction
        \/ SnapshotRecoveredAction
        \/ ReclaimExpiredAction
        \/ Done

TypeOK == /\ now \in 0..Horizon
          /\ phase \in [Requests -> Phases]
          /\ origin \in [Requests -> Replicas \union {"none"}]
          /\ deadline \in [Requests -> 0..Horizon + LeaseTTL]
          /\ waitDeadline \in [Requests -> 0..Horizon + WaitBudget]
          /\ generation \in [Requests -> 0..Cardinality(Requests)]
          /\ delivery \in [Requests -> {"none", "delivered", "lost"}]
          /\ upstream \in [Requests -> {"none", "sent"}]
          /\ upstreamStart \in [Requests -> 0..Horizon]
          /\ finishedAt \in [Requests -> 0..Horizon]
          /\ recovery \in [Requests -> {"not_required", "required", "recovered"}]
          /\ reservedCount \in 0..MaxActive
          /\ Cardinality(WaitingTickets) <= MaxWaiting

ActiveLeaseSafety == /\ reservedCount = Cardinality(UnreclaimedLeases)
                     /\ reservedCount <= MaxActive
                     /\ \A request \in Requests :
                          /\ phase[request] = "active" =>
                               /\ now < deadline[request]
                               /\ generation[request] > 0
                               /\ origin[request] # "none"
                          /\ upstream[request] = "sent" =>
                               upstreamStart[request] + FencingMargin < deadline[request]

ConfirmedReleaseInsideLease == \A request \in Requests :
    phase[request] = "completed" => finishedAt[request] < deadline[request]

NoEarlyReuse == \A request \in Requests :
    phase[request] = "expiredPending" => now < deadline[request] + ReclaimGrace

SnapshotRecoverySafety == \A request \in Requests :
    recovery[request] = "required" => upstream[request] = "none"


=============================================================================
