# OpenAPIBudgetLeasesChartBrokenStaleStart

TLC reached 136 states and 182 steps. The chart draws 7 states and 8 arrows.
The chart does not draw 84 steps that stay in one chart state.
The view hides 38 steps, so the chart does not draw them.

## LeaseSafety breaks
The shortest path has 5 steps:
0. No request yet
1. a replica takes a bounded queue ticket: Bounded queue ticket waiting
2. the shared quota admits one request: Lease held, upstream not sent
3. time advances and a due lease expires: Lease held, upstream not sent
4. time advances and a due lease expires: Lease held, upstream not sent
5. a Worker starts the only upstream attempt before the fence: Lease held, upstream already sent

## Rules that hold
TypeOK, ConfirmedReleaseInsideLease, NoDeadlock
