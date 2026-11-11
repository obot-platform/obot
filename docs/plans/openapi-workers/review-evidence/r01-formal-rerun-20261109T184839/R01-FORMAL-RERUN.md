# R01 formal rerun summary

- UTC start/end: 2026-11-09T10:49:28Z / 2026-11-09T10:50:07Z
- TLC artifact: /tmp/tla2tools-1.8.0.jar, SHA-256 e1f11b75c4b5bc4478ba0ac5cb6ae28e40a01b0016bc33e0aa97da22dfdbcd7f
- Models, configs, scripts, and outputs were copied/generated under this directory; source feature formal artifacts were not modified.

## Script results

f01 exit=0 elapsed=8s log=docs/plans/openapi-workers/review-evidence/r01-formal-rerun-20261109T184839/logs/f01.log
f02 exit=0 elapsed=4s log=docs/plans/openapi-workers/review-evidence/r01-formal-rerun-20261109T184839/logs/f02.log
f03 exit=0 elapsed=7s log=docs/plans/openapi-workers/review-evidence/r01-formal-rerun-20261109T184839/logs/f03.log
f04 exit=0 elapsed=12s log=docs/plans/openapi-workers/review-evidence/r01-formal-rerun-20261109T184839/logs/f04.log
f05 exit=0 elapsed=8s log=docs/plans/openapi-workers/review-evidence/r01-formal-rerun-20261109T184839/logs/f05.log

## Report/chart assertions

- `f01/docs/plans/openapi-workers/formal/evidence/base/report.md`: 33 states and 56 steps. The chart draws 10 states and 9 arrows. | holding=TypeOK, SnapshotSuppliedAtMostOnce, ExecutionEnteredAtMostOnce, UpstreamAttemptedAtMostOnce, UpstreamRequiresOneSnapshotAndOneFence, SnapshotRequiredNeverTouchesUpstream, NoRetryAfterUncertainOutcome, NoDeadlock | breaks=none
- `f01/docs/plans/openapi-workers/formal/evidence/broken/report.md`: 57 states and 120 steps. The chart draws 13 states and 14 arrows. | holding=TypeOK, SnapshotSuppliedAtMostOnce, UpstreamAttemptedAtMostOnce, SnapshotRequiredNeverTouchesUpstream, NoDeadlock | breaks=## ExecutionEnteredAtMostOnce breaks;## UpstreamRequiresOneSnapshotAndOneFence breaks;## NoRetryAfterUncertainOutcome breaks;
- `f02/docs/plans/openapi-workers/formal/statechart-broken/report.md`: 19 states and 22 steps. The chart draws 9 states and 8 arrows. | holding=TypeOK, NoDeadlock, CHECK_DEADLOCK | breaks=## ExactCallerCredentialOnly breaks;
- `f02/docs/plans/openapi-workers/formal/statechart-pass/report.md`: 18 states and 21 steps. The chart draws 10 states and 9 arrows. | holding=ExactCallerCredentialOnly, TypeOK, NoDeadlock, CHECK_DEADLOCK | breaks=none
- `f03/docs/plans/openapi-workers/formal/runs/latest/statechart-broken/report.md`: 20 states and 35 steps. The chart draws 7 states and 11 arrows. | holding=RawRequestSemanticsPreservedOnExecution, RejectionNeverCallsUpstream, NoDeadlock | breaks=## ExecutionRequiresUniqueRoute breaks;## ExecutionRequiresWhitelist breaks;
- `f03/docs/plans/openapi-workers/formal/runs/latest/statechart-normal/report.md`: 17 states and 29 steps. The chart draws 7 states and 9 arrows. | holding=ExecutionRequiresUniqueRoute, ExecutionRequiresWhitelist, RawRequestSemanticsPreservedOnExecution, RejectionNeverCallsUpstream, NoDeadlock | breaks=none
- `f04/docs/plans/openapi-workers/formal/statechart-broken-early/report.md`: 139 states and 191 steps. The chart draws 7 states and 9 arrows. | holding=TypeOK, LeaseSafety, NoDeadlock | breaks=## ConfirmedReleaseInsideLease breaks;
- `f04/docs/plans/openapi-workers/formal/statechart-broken-stale/report.md`: 136 states and 182 steps. The chart draws 7 states and 8 arrows. | holding=TypeOK, ConfirmedReleaseInsideLease, NoDeadlock | breaks=## LeaseSafety breaks;
- `f04/docs/plans/openapi-workers/formal/statechart-chart/report.md`: 115 states and 152 steps. The chart draws 7 states and 7 arrows. | holding=TypeOK, LeaseSafety, ConfirmedReleaseInsideLease, NoDeadlock | breaks=none
- `f05/docs/plans/openapi-workers/formal/statechart-base/report.md`: 65,706 states, more than the chart budget of 500, so the command wrote no chart.mmd. | holding=TypeOK, NoDeadlock, OpenAPITrafficGated, PlannedRollRetainsCapacity, CHECK_DEADLOCK | breaks=none
- `f05/docs/plans/openapi-workers/formal/statechart-broken-chart/report.md`: 64 states and 158 steps. The chart draws 10 states and 28 arrows. | holding=TypeOK, NoDeadlock, CHECK_DEADLOCK | breaks=## OpenAPITrafficGated breaks;## PlannedRollRetainsCapacity breaks;
- `f05/docs/plans/openapi-workers/formal/statechart-chart/report.md`: 37 states and 71 steps. The chart draws 8 states and 13 arrows. | holding=TypeOK, NoDeadlock, OpenAPITrafficGated, PlannedRollRetainsCapacity, CHECK_DEADLOCK | breaks=none

## Unchecked-rule scan

No copied statechart report contains `Rules not checked`.
