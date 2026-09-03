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

Every Obot replica holds a kinm watch on each of its object types, about 44 of them. When
nothing has woken a watch, it lists its table again every 2 seconds. That poll is the only way
one replica learns about a write made by another, because kinm's in process broadcast reaches
only the process that did the write.

The cost is paid whether or not anything is happening. Ten idle environments with one replica
each cost about 1,000 statements per second on one shared Cloud SQL instance. With two replicas
each they cost about 1,960, and database CPU doubled with them, because the standby keeps its
caches warm and so polls exactly as hard as the leader.

## Decision

Writes announce themselves and watches wait to be told, instead of asking.

kinm runs `pg_notify` inside every writing transaction, naming the table. Each process holds one
dedicated Postgres connection that listens for those notifications and, when one names a table,
calls the same broadcast the in process write path already calls. Everything downstream of that
broadcast is unchanged. A listener only reports itself connected once a notification it sends
itself has come back, so a connection that cannot deliver notifications does not count as one.

While the listener is connected, a watch that nothing has woken lists again every 2 minutes
rather than every 2 seconds. Whenever the listener is not connected, the 2 second poll returns.
Notifications for one table are combined over 1 second; the first after a quiet moment passes
straight through, and the rest collapse into one wake up at the end of the second.

Obot keeps the kinm `Factory` on `Services` and calls `Factory.Refresh` from the post start hook
that nah runs on promotion to leader. Refresh wakes every watch in the process so each lists once
before the new leader acts on the cache it had as a standby.

## Rationale

Notifications rather than cheaper polling, because polling a smaller table would have shrunk the
idle cost but kept it proportional to the number of replicas, and its interval could not have been
lengthened without giving up change detection latency. With notifications carrying the changes,
the poll can be long.

The poll is kept rather than removed, because a replica on the previous version emits nothing.
During a rolling upgrade, a replica already on the new code would otherwise stay stale on any
table the old replica wrote until that table happened to be written again. The 2 minute poll
bounds that with no coordination between deploys.

Notifications are combined over 1 second so that a continuously written table costs every other
replica at most one list per second, against one every 2 seconds today, and no table becomes more
expensive than it was.

Watches are refreshed on promotion because nah starts its informer cache once and does not list
again when a standby becomes the leader. Whatever the standby had missed, the leader would then
act on. One list per type, once per promotion, closes that.

## Consequences

Idle statement load from watches falls about 19x, measured at 196 statements per second per
environment on the previous code against 10.1 with this change. Most of what remains is the
leader election lease, addressed separately in nah.

A change made on one replica reaches the other in about 17ms when the table was quiet, and within
1 second otherwise. Obot's controllers write a finalizer after every create, so steady writes to a
controlled type see about 500ms. The bound today is 2 seconds.

Each replica holds one more Postgres connection, named `kinm-listener` in `pg_stat_activity`.

The first rolling deploy of this version has a window, once per environment, where an upgraded
replica can be up to 2 minutes behind a replica still on the old code. It closes on its own.

`KINM_DB_DISABLE_NOTIFY=true` restores the previous behavior exactly. `KINM_DB_WATCH_POLL_SECONDS`
and `KINM_DB_NOTIFY_DEBOUNCE_MILLISECONDS` tune the two intervals.

## References

- https://github.com/obot-platform/kinm/pull/29
- https://github.com/obot-platform/obot/pull/7736
- kinm `pkg/db/listener.go`, `pkg/db/strategy.go`
- `pkg/controller/controller.go`, `PostStart`
