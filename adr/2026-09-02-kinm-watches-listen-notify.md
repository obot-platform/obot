# 2026-09-02: Wake kinm watches with Postgres LISTEN/NOTIFY and refresh them on promotion

- **Status:** Accepted
- **Date:** 2026-09-02
- **Supersedes:** None
- **Superseded by:** None

## Related issues

None.

## Related ODPs

None.

## Context

Every Obot replica held a kinm watch on each of its object types, about 44 of them. A watch that
nothing had woken listed its table again every 2 seconds. The poll was the only way one replica
learned about a write made by another.

The cost was the same whether or not anything was happening. Ten idle environments with one
replica each cost about 1,000 statements per second on one shared Cloud SQL instance. With two
replicas each they cost about 1,960, and database CPU doubled along with it. The standby kept its
caches warm, so it polled exactly as hard as the leader.

## Decision

Change how a replica learns about another replica's writes, from polling every 2 seconds to
Postgres LISTEN/NOTIFY. kinm runs `pg_notify` inside every writing transaction, naming the table.
Each replica holds one dedicated Postgres connection that listens for those notifications. When
one arrives, the listener wakes the watches on that table the same way a local write does, so
nothing downstream changes. A listener does not count as connected until a notification it sent
itself comes back.

The poll becomes a backup for when notifications do not arrive, and drops from every 2 seconds to
every 2 minutes. If the listener is not connected, polling is all a replica has, so it goes back
to every 2 seconds.

Notifications for one table are combined over 1 second. The first one wakes the watches straight
away, and any that follow within the second collapse into a single wake up at the end of it.

When a replica is promoted to leader, refresh every watch. Obot calls `Factory.Refresh` from the
post start hook that nah runs on promotion, so every watch lists again rather than carrying a
standby's staleness into the leader. nah starts the controllers before it runs post start hooks,
and Refresh does not wait for the lists it triggers, so a controller can run briefly against the
older cache before the new one arrives.

## Rationale

A replica no longer asks the database whether anything changed. An idle environment costs nothing
for its watches, however many replicas it runs.

The poll has to stay because a replica running the previous version does not announce its writes.
During a rolling upgrade a replica already on the new code would otherwise miss those writes until
something touched the same table again. Two minutes puts a ceiling on how long that lasts, and
needs no coordination between deploys.

One second bounds what a busy table costs every other replica. However fast a table is written,
its watches elsewhere wake at most once a second, so a burst of writes on one replica cannot turn
into a burst of queries on all of them.

The refresh on promotion is needed because nah starts its cache once and never lists again when a
standby becomes leader. Whatever the standby missed, the new leader would act on.

## Consequences

Idle statement load from watches falls about 19x, from 196 statements per second per environment
to 10.1. Most of what is left is the leader election lease, which nah handles separately.

A change made on one replica reaches the other in milliseconds if the table was quiet, or within a
second if it was busy. Before, it took up to 2 seconds.

A table that is written continuously costs the other replicas about twice what it did, one wake up
a second against one every 2 seconds, in exchange for the idle case costing nothing.

Each replica holds one more Postgres connection, named `kinm-listener` in `pg_stat_activity`.

On the first deploy of this version an upgraded replica can run up to 2 minutes behind one still
on the old code. The lag lasts until that replica is upgraded too, and only happens on this one
deploy.

`KINM_DB_DISABLE_NOTIFY=true` restores the previous behavior exactly. `KINM_DB_WATCH_POLL_SECONDS`
and `KINM_DB_NOTIFY_DEBOUNCE_MILLISECONDS` tune the two intervals.

## References

- https://github.com/obot-platform/kinm/pull/29
- https://github.com/obot-platform/obot/pull/7736
- kinm `pkg/db/listener.go`, `pkg/db/strategy.go`
- `pkg/controller/controller.go`, `PostStart`
