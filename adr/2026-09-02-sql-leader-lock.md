# 2026-09-02: Hold the controller leader lock in a SQL table instead of a Lease

- **Status:** Accepted
- **Date:** 2026-09-02
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None. Implemented in https://github.com/obot-platform/obot/pull/7727 and
https://github.com/obot-platform/nah/pull/32.

## Related ODPs

None.

## Context

Obot ran its controllers on one replica at a time, chosen by client-go leader
election. The lock was a `coordination.k8s.io` Lease object stored in kinm, Obot's
own API server backed by Postgres.

kinm stored objects in versioned, append-only tables. The leader rewrote the Lease
every two seconds, so between compactions, which ran every fifteen minutes, the Lease
had about 450 row versions, and Postgres sorted all of them on every read. When every
Obot Cloud environment moved from one replica to two on 2026-08-20, database CPU on
the shared instance rose and stayed up, and reading the Lease was the query with
the most execution time on that instance.

## Decision

Change where the `obot-controller` election stores its lock, from a Lease in kinm's
versioned tables to one row in a `leader_lock` table in the same Postgres database.
Read the row by primary key and update it in place with a version check. Keep
client-go's election algorithm, TTL, and retry period as they are. The lock type is
selectable in nah as `ResourceLockType` `sql`, next to the existing file lock.

Until we deem it safe to remove, we are also introducing a `WithLegacyLeaseLock` option
on the election config, which points the new lock at the Lease the election used
before. While the `leader_lock` table has no row, a replica follows that legacy Lease,
which replicas on the previous release still hold. After the last of them releases the
Lease and five seconds pass, one replica creates the row, and from then on no replica
reads the Lease. This is to make the rolling upgrade that transitions from the old to
the new paradigm smooth. The option can be removed in a future release.

## Rationale

This is one effort in a series of efforts to reduce the load an idle Obot puts on its
database. Metrics show that the query that reads the Lease is by far the busiest query in
most Obot Cloud environments. This eliminates that and is objectively a better fit for
leader election in general. We already had a file based locking mechanism, so this was
an easy extension to the existing framework.

While this will remove the most expensive query (by volume and execution time) in the Cloud
environment, it won't solve the broader DB CPU consumption problem in general as that
is largely a factor of the sheer volume of queries kinm makes in order to watch all of
Obot's types.

## Consequences

As mentioned above, this will cut out the reads and writes of the Lease kinm object.

The lock creates the `leader_lock` table at startup if it is missing. The old Lease
row stays behind in the `lease` table, unread and unwritten, until a later release
removes the leases API group.

Failover behavior is unchanged, which we verified locally against Postgres and in the
main environment.

During the transition we have a five second grace period, so a replica on the new
release does not create the row the instant the legacy Lease is released. Replicas
still on the old release poll every two seconds, so the wait gives them a chance to
claim the Lease first and keeps the election on one lock while any of them is still
running. If one of them is stalled longer than that at the moment the last old leader
releases, it can take the Lease after the row already exists, and both lead until the
rollout terminates it. We accepted that window rather than deploying the one release
with a `Recreate` strategy.

## References

- nah SQL lock: `pkg/leader/locks/sql.go` and `pkg/leader/leader.go` in
  https://github.com/obot-platform/nah
- Obot wiring: `pkg/services/config.go`
