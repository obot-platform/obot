# 2026-11-04: Incremental scheduled audit-log exports

- **Status:** Accepted
- **Date:** 2026-11-04
- **Supersedes:** None
- **Superseded by:** None

## Related issues

- [Incremental scheduled exports, issue 8419](https://github.com/obot-platform/obot/issues/8419)

## Related ODPs

None. This extends the existing export controllers and storage resources.

## Context

Repeated lookback exports duplicate large amounts of history and can miss later responses to old MCP requests or delayed local-agent events when selected by event time.

## Decision

Use GORM-managed update timestamps for scheduled MCP, local-agent, and LLM exports, falling back to creation time for historical rows. Keep consumer event timestamps and one-time export selection unchanged. Select each fixed time window through ordinary update-time and creation-time indexes, then paginate by immutable ID so rows moving out of the window cannot shift unrelated rows past an offset. ID ordering controls pagination only; coverage between exports remains timestamp-based.

Persist each schedule's pending window and captured configuration before creating its deterministic export job. Retry that job with bounded cadence and a stable object key. Advance the schedule's exported-through cursor only after the job's successful completion is persisted. The first window uses the configured lookback; subsequent windows overlap the cursor by five seconds.

## Rationale

Server write times include delayed submissions and later updates. A short overlap accommodates brief write/commit delays without adding an outbox or delivery ledger. Keeping progress separate from run time prevents failed uploads from consuming coverage and permits recovery after restarts or disabling a schedule.

## Consequences

Delivery is best effort. Writes invisible for longer than five seconds may be missed. Overlap and retries can duplicate records, and updated versions retain their event IDs. Long outages can recover only retained data. Configuration edits preserve coverage and do not replay history.

## References

- [Scheduled export controller](../pkg/controller/handlers/scheduledauditlogexport/scheduledauditlogexport.go)
- [Audit data documentation](../docs/docs/security/audit-data.md)
