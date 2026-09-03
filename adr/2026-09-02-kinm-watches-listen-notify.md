# 2026-09-02: Wake kinm watches with Postgres LISTEN/NOTIFY and refresh them on promotion

- **Status:** Accepted
- **Date:** 2026-09-02
- **Supersedes:** None
- **Superseded by:** None

## Related issues

https://github.com/obot-platform/obot/issues/7737

## Related ODPs

None.

## Context

Every Obot replica holds a kinm watch on each of its object types, about 44 of them. A watch
that nothing has woken lists its table again every 2 seconds. That poll is the only way one
replica learns about a write made by another, because kinm's in process broadcast reaches only
the process that did the write. With two replicas per environment on one shared Cloud SQL
instance, ten idle environments cost about 1,960 statements per second, and moving to two
replicas roughly doubled database CPU, because the standby keeps its caches warm and so polls
exactly as hard as the leader.

## Decision

kinm announces every write with `pg_notify` inside the writing transaction, and each process
holds one dedicated connection that listens on a single channel and wakes the same broadcast
the in process write path uses. While that listener is connected the poll stretches to 2
minutes and stays as a fallback; without one it remains at 2 seconds. Notifications for one
table are combined over 1 second, with the first after a quiet moment never held back. A new
listening connection sends itself a notification and waits for it before reporting connected.

Obot keeps the kinm `Factory` on `Services` and calls `Factory.Refresh` from the post start
hook, which nah runs on promotion to leader, so a newly promoted leader lists every type once
before acting on the cache it had as a standby.

## Rationale

Polling a summary table instead would have shrunk the idle cost but kept it proportional to
replicas, and its interval could not be lengthened without giving up change detection latency.
Notifications remove the idle term and leave the poll free to be long.

The poll stays because a replica on the previous version emits nothing. During a rolling
upgrade a replica already on the new code would otherwise stay stale on any table the old one
wrote until that table happened to be written again. The poll bounds that at 2 minutes with no
coordination between deploys.

The debounce is 1 second rather than shorter so that a continuously written table costs every
other replica one list per second, against one every 2 seconds today, and no table becomes more
expensive than it was. At 100ms the same table would have cost ten a second.

nah starts its informer cache once and does not list again on promotion, so a standby's
staleness would become the leader's. One list per type, once per promotion, is cheap.

`LISTEN` succeeding does not mean notifications arrive. A pooler in transaction mode accepts it
and delivers nothing, and without the probe every watch would sit on the long poll while looking
healthy.

## Consequences

Idle statement load from watches falls about 15x, measured at 196 statements per second per
environment on the previous code against 12.7 with a one minute poll and 10.1 with the two
minute poll. Most of what remains is the leader election lease, addressed separately in nah.

A change made on one replica reaches the other in about 17ms when the table was quiet, and
within 1 second otherwise. Obot's controllers write a finalizer after every create, so steady
writes to a controlled type see about 500ms. The bound today is 2 seconds.

Each replica holds one more Postgres connection, named `kinm-listener` in `pg_stat_activity`.

The first rolling deploy of this version has a window, once per environment, where an upgraded
replica can be up to 2 minutes behind a replica still on the old code. It closes on its own.

`KINM_DB_DISABLE_NOTIFY=true` restores the previous behavior exactly, and `KINM_DB_WATCH_POLL_SECONDS`
and `KINM_DB_NOTIFY_DEBOUNCE_MILLISECONDS` tune the two intervals.

## References

- https://github.com/obot-platform/kinm/pull/29
- https://github.com/obot-platform/obot/pull/7736
- kinm `pkg/db/listener.go`, `pkg/db/strategy.go`
- `pkg/controller/controller.go`, `PostStart`
