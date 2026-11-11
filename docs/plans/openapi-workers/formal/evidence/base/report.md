# SnapshotFencing

TLC reached 33 states and 56 steps. The chart draws 10 states and 9 arrows.
The chart does not draw 18 steps that stay in one chart state.
The view hides 20 steps, so the chart does not draw them.

## Rules that hold
TypeOK, SnapshotSuppliedAtMostOnce, ExecutionEnteredAtMostOnce, UpstreamAttemptedAtMostOnce, UpstreamRequiresOneSnapshotAndOneFence, SnapshotRequiredNeverTouchesUpstream, NoRetryAfterUncertainOutcome, NoDeadlock
