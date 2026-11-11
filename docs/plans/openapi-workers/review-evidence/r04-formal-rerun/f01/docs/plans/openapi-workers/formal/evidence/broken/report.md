# SnapshotFencing

TLC reached 57 states and 120 steps. The chart draws 13 states and 14 arrows.
The chart does not draw 33 steps that stay in one chart state.
The view hides 36 steps, so the chart does not draw them.

## ExecutionEnteredAtMostOnce breaks
The shortest path has 8 steps:
0. Request not seen
1. first execute sees the cache: Cold cache observed
2. worker asks for the snapshot: Caller told to send snapshot
3. caller sends the snapshot once: Snapshot compiling
4. compiled plan becomes ready: Snapshot ready, fence unused
5. worker takes the one-shot fence: Upstream execution in flight
6. worker starts the business API call: Upstream execution in flight
7. business response returns: Business result returned
8. same request enters execution again: Execution entered again

## UpstreamRequiresOneSnapshotAndOneFence breaks
The shortest path has 8 steps:
0. Request not seen
1. first execute sees the cache: Cold cache observed
2. worker asks for the snapshot: Caller told to send snapshot
3. caller sends the snapshot once: Snapshot compiling
4. compiled plan becomes ready: Snapshot ready, fence unused
5. worker takes the one-shot fence: Upstream execution in flight
6. worker starts the business API call: Upstream execution in flight
7. business response returns: Business result returned
8. same request enters execution again: Execution entered again

## NoRetryAfterUncertainOutcome breaks
The shortest path has 9 steps:
0. Request not seen
1. first execute sees the cache: Cold cache observed
2. worker asks for the snapshot: Caller told to send snapshot
3. caller sends the snapshot once: Snapshot compiling
4. compiled plan becomes ready: Snapshot ready, fence unused
5. worker takes the one-shot fence: Upstream execution in flight
6. worker starts the business API call: Upstream execution in flight
7. business response returns: Business result returned
8. same request enters execution again: Execution entered again
9. transport outcome becomes uncertain: Transport outcome uncertain after duplicate entry

## Rules that hold
TypeOK, SnapshotSuppliedAtMostOnce, UpstreamAttemptedAtMostOnce, SnapshotRequiredNeverTouchesUpstream, NoDeadlock
