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
had about 450 row versions, and Postgres sorted all of them on every read. Each
replica read the Lease once every two seconds: the follower on its poll, and the
leader inside its renew. When every Obot Cloud environment moved from one replica to
two on 2026-08-20, database CPU on the shared instance rose and stayed up, and the
Lease was the query with the most execution time on that instance.

## Decision

Change where the `obot-controller` election stores its lock, from a Lease in kinm's
versioned tables to one row in a `leader_lock` table in the same Postgres database.
Read the row by primary key and update it in place with a version check. Keep
client-go's election algorithm, TTL, and retry period as they are. The lock type is
selectable in nah as `ResourceLockType` `sql`, next to the existing file lock.

In the release that introduces the change, enable `WithLegacyLeaseLock`. While the
`leader_lock` table has no row, a replica follows the Lease that replicas on the
previous release still hold. After the last of them releases the Lease and five
seconds pass, one replica creates the row, and from then on no replica reads the
Lease. Remove the option in the release after this one.

## Rationale

A row read by primary key and updated in place has one version, so the cost of each
read is the same on every poll and does not grow between compactions. The version
check on update gives the same guarantee client-go's Lease lock gives: a replica can
only write over a record it has read, so two replicas cannot both hold the lock.

A rolling update runs replicas on the old release and the new release side by side.
If the new replicas did not read the Lease, each release would elect its own leader
and the controllers would run twice until the last old replica was gone. Replicas on
the old release poll every two seconds and take a released Lease on their next poll,
so a five second wait lets them win any race and keeps the election on one lock until
they are gone.

## Consequences

Every two seconds the election costs one primary key read per replica, plus one
single-row update by the leader. On the first environment to receive the change, the
two Lease queries went from 10% of sampled query execution time to none.

Failover is unchanged. In local runs against Postgres, the leader released the lock
84 to 95 ms after SIGTERM, a follower took over within three seconds, and a crashed
leader was replaced 60 to 63 seconds after it died, once the 60 second TTL passed.

The `leader_lock` table is not one of kinm's objects and is not visible through the
Kubernetes-style API. The lock creates the table at startup if it is missing.

The old Lease object remains in kinm's `lease` table. Nothing reads or writes it, and
kinm continues to compact that table at two statements per fifteen minutes per
replica, until a later release removes the leases API group.

During the one rolling update that introduces the change, the five second wait is a
timing argument, not a guarantee. A replica on the old release that stalls for more
than five seconds at the moment the last old leader releases can take the Lease after
a new replica has created the row, and both lead until the rollout terminates the old
replica. The team accepted the window. Running that one deploy with a `Recreate`
strategy removes it.

Each replica logs at startup which lock its election uses. While it follows a Lease,
it logs once who holds it, once when the five second wait starts, and once when it
creates the row.

## References

- nah SQL lock: `pkg/leader/locks/sql.go` and `pkg/leader/leader.go` in
  https://github.com/obot-platform/nah
- Obot wiring: `pkg/services/config.go`
