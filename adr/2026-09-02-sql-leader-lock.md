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

Obot runs its controllers on one replica at a time, chosen by leader election. The
election is client-go's, and until this change its lock was a `coordination.k8s.io`
Lease object stored in kinm, Obot's own API server backed by Postgres.

kinm stores objects in versioned, append-only tables. A lock is rewritten every
renew period, two seconds, so between compactions the Lease accumulated hundreds
of row versions, and every read of it sorted through all of them. Both replicas of
an HA environment paid that cost once per period, the follower through its poll and
the leader through the read inside its renew. When every Obot Cloud environment
moved to two replicas on 2026-08-20, database CPU on the shared instance rose and
stayed up, and the Lease became the single largest consumer of database time there.
Its cost grew with the renew period, the compaction interval, and the replica count.

## Decision

nah gains a `sql` `ResourceLockType`: the leader election record lives in one row
of a plain `leader_lock` table, read by primary key and updated in place with a
version check, exactly the contract client-go's `LeaseLock` provides. Obot's
`obot-controller` election uses it against the same database kinm uses, through
`leader.NewSQLElectionConfig`. The election algorithm, TTL, and retry period are
unchanged.

The `obot-local-controller` election, which manages resources in the real
Kubernetes cluster, keeps using a real Lease in that cluster.

The release that introduces the change carries a bridge, `WithLegacyLeaseLock`.
While the `leader_lock` table has no row, a replica follows the Lease that replicas
on the previous release still hold, and creates the row only after the last of them
has released it and a five second grace period has passed. Once the row exists the
Lease is never read again. The bridge is removed in the release after this one.

## Rationale

The cost was structural: a lock that is rewritten every two seconds does not belong
in an append-only versioned store. Making that store cheaper per read (an index on
the latest-version lookup, a shorter compaction interval) reduces the constant but
leaves the cost proportional to replicas and poll rate, and leaves the Lease as the
top query. Slowing the election trades away failover speed that nah's hot-standby
design was built for. A plain table makes a read a point lookup and a renew a
single-row update, so the cost no longer depends on replicas, poll period, or
compaction. Keeping the lock pluggable in nah, next to the existing file lock, keeps
nah generic and leaves the real-cluster election on the tool that suits it.

The bridge exists because the switch ships through a rolling update in which old and
new replicas overlap, and without it each side would elect a leader on its own lock.

## Consequences

The database cost of the election is now constant and small. Failover behavior is
unchanged: the lock is released within about 100 ms of SIGTERM, a follower takes over
within one to three seconds, and a crashed leader is replaced after the 60 second TTL.

The `leader_lock` table sits outside kinm's object model and is created by the lock
on first use, so it is not visible through the Kubernetes-style API. The old Lease
object remains in kinm's `lease` table until the leases API group is removed in a
later release; it is no longer read or written, and its compaction cost is negligible.

During the one rolling update that introduces the change, the grace period is a
timing argument rather than a guarantee: an old follower stalled for more than five
seconds at the moment the last old leader releases could lead alongside a new
replica until the rollout terminates it. This was accepted knowingly. Running that
one deploy with a `Recreate` strategy would remove the window.

Operators can read the state from the logs. Each replica logs which lock its election
uses at startup, and the bridge logs, once per transition, that it is following a
legacy leader, that the grace period started, and that the takeover happened.

## References

- nah SQL lock: `pkg/leader/locks/sql.go` and `pkg/leader/leader.go` in
  https://github.com/obot-platform/nah
- Obot wiring: `pkg/services/config.go`
- Related kinm changes considered alongside this one:
  https://github.com/obot-platform/kinm/pull/24 (index) and
  https://github.com/obot-platform/kinm/pull/25 (compaction interval)
