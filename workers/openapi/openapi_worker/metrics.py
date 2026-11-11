"""Bounded Prometheus text-format metrics for the operations dashboard.

Design constraints:
- No new runtime dependencies; Prometheus text exposition format v0.0.4 is hand-rendered.
- Label cardinality is bounded: every label value comes from a fixed vocabulary
  (route names, WorkerError codes, HTTP status classes, booleans), never from
  request IDs, digests, URLs, header names, or other caller-controlled values.
- The rendered body is bounded: request counters are reset on scrape, so the
  payload is proportional to the fixed code/status vocabularies, not to traffic.
"""

from __future__ import annotations

import time

from .errors import WorkerError

_ROUTES = ("describe", "execute", "healthz", "readyz")
_UNSPECIFIED = "unspecified"


class _Summary:
    """Fixed-bucket cumulative histogram (seconds)."""

    BUCKETS = (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0)

    def __init__(self, name: str, help_text: str, labels: tuple[str, ...]) -> None:
        self.name = name
        self.help = help_text
        self.labels = labels
        self.bucket_counts: dict[tuple[str, ...], list[int]] = {}
        self.sums: dict[tuple[str, ...], float] = {}
        self.counts: dict[tuple[str, ...], int] = {}

    def observe(self, value: float, label_values: tuple[str, ...]) -> None:
        buckets = self.bucket_counts.setdefault(label_values, [0] * len(self.BUCKETS))
        for index, edge in enumerate(self.BUCKETS):
            if value <= edge:
                buckets[index] += 1
        self.sums[label_values] = self.sums.get(label_values, 0.0) + value
        self.counts[label_values] = self.counts.get(label_values, 0) + 1

    def render(self) -> list[str]:
        lines: list[str] = []
        for label_values in sorted(self.bucket_counts):
            labels = self._render_labels(label_values)
            buckets = self.bucket_counts[label_values]
            cumulative = 0
            for edge, count in zip(self.BUCKETS, buckets):
                cumulative += count
                lines.append(f"{self.name}_bucket{self._with_le(labels, edge)} {cumulative}")
            lines.append(f"{self.name}_bucket{self._with_le(labels, '+Inf')} {self.counts.get(label_values, 0)}")
            lines.append(f"{self.name}_sum{labels} {self.sums.get(label_values, 0.0):.6f}")
            lines.append(f"{self.name}_count{labels} {self.counts.get(label_values, 0)}")
        return lines

    def _render_labels(self, label_values: tuple[str, ...]) -> str:
        pairs = [f'{name}="{value}"' for name, value in zip(self.labels, label_values[: len(self.labels)])]
        return "{" + ", ".join(pairs) + "}" if pairs else ""

    def _with_le(self, labels: str, edge) -> str:
        rendered = f'le="{edge:g}"' if isinstance(edge, float) else f'le="{edge}"'
        if labels:
            return "{" + labels[1:-1] + ", " + rendered + "}"
        return "{" + rendered + "}"


class WorkerMetrics:
    """Process-lifetime gauges plus scrape-reset request counters."""

    def __init__(self) -> None:
        self.start_time = time.time()
        self._route_requests: dict[tuple[str, str], int] = {}
        self._route_errors: dict[tuple[str, str, str], int] = {}
        self._upstream_results: dict[tuple[str, str], int] = {}
        self._snapshot_events: dict[tuple[str, str], int] = {}
        self._compile_durations = _Summary("openapi_worker_compilation_seconds", "Cold snapshot compilation wall time", ("outcome",))
        self._route_durations = _Summary("openapi_worker_request_seconds", "Internal route handling wall time", ("route",))

    # Recording API. All label values must come from fixed vocabularies.
    def record_route(self, route: str, outcome: str) -> None:
        self._route_requests[(route, outcome)] = self._route_requests.get((route, outcome), 0) + 1

    def record_route_duration(self, route: str, seconds: float) -> None:
        self._route_durations.observe(seconds, (route,))

    def record_error(self, route: str, code: str) -> None:
        if code not in _KNOWN_ERROR_CODES:
            code = _UNSPECIFIED
        self._route_errors[(route, code)] = self._route_errors.get((route, code), 0) + 1

    def record_upstream(self, status: int, outcome: str) -> None:
        status_class = f"{status // 100}xx" if 100 <= status <= 599 else "0xx"
        self._upstream_results[(status_class, outcome)] = self._upstream_results.get((status_class, outcome), 0) + 1

    def record_snapshot_event(self, event: str) -> None:
        self._snapshot_events[event] = self._snapshot_events.get(event, 0) + 1

    def record_compilation(self, seconds: float, outcome: str) -> None:
        self._compile_durations.observe(seconds, (outcome,))

    def render(self, registry, active_references: int) -> str:
        lines: list[str] = []
        lines.append("# HELP openapi_worker_up Whether this worker process answered the last scrape.")
        lines.append("# TYPE openapi_worker_up gauge")
        lines.append("openapi_worker_up 1")
        lines.append("# HELP openapi_worker_start_time_seconds Unix time when the worker process started.")
        lines.append("# TYPE openapi_worker_start_time_seconds gauge")
        lines.append(f"openapi_worker_start_time_seconds {self.start_time:.2f}")
        lines.append("# HELP openapi_worker_snapshot_cache_entries Cached compiled snapshots.")
        lines.append("# TYPE openapi_worker_snapshot_cache_entries gauge")
        lines.append(f"openapi_worker_snapshot_cache_entries {len(registry.entries)}")
        lines.append("# HELP openapi_worker_snapshot_cache_bytes Cached schema bytes.")
        lines.append("# TYPE openapi_worker_snapshot_cache_bytes gauge")
        lines.append(f"openapi_worker_snapshot_cache_bytes {registry.bytes}")
        lines.append("# HELP openapi_worker_snapshot_cache_limit_entries Configured snapshot cache entry limit.")
        lines.append("# TYPE openapi_worker_snapshot_cache_limit_entries gauge")
        lines.append(f"openapi_worker_snapshot_cache_limit_entries {registry.cache_entries}")
        lines.append("# HELP openapi_worker_snapshot_cache_limit_bytes Configured snapshot cache byte limit.")
        lines.append("# TYPE openapi_worker_snapshot_cache_limit_bytes gauge")
        lines.append(f"openapi_worker_snapshot_cache_limit_bytes {registry.cache_bytes}")
        lines.append("# HELP openapi_worker_snapshot_active_references Snapshots with in-flight executions.")
        lines.append("# TYPE openapi_worker_snapshot_active_references gauge")
        lines.append(f"openapi_worker_snapshot_active_references {active_references}")
        lines.append("# HELP openapi_worker_pending_compilations In-flight or queued cold compilations.")
        lines.append("# TYPE openapi_worker_pending_compilations gauge")
        lines.append(f"openapi_worker_pending_compilations {len(registry.pending)}")
        lines.append("# HELP openapi_worker_compiles_total Cumulative cold compilations started.")
        lines.append("# TYPE openapi_worker_compiles_total counter")
        lines.append(f"openapi_worker_compiles_total {registry.compile_count}")

        lines.append("# HELP openapi_worker_snapshot_events_total Snapshot lifecycle events since process start.")
        lines.append("# TYPE openapi_worker_snapshot_events_total counter")
        for event in sorted(self._snapshot_events):
            lines.append(f'openapi_worker_snapshot_events_total{{event="{event}"}} {self._snapshot_events[event]}')

        lines.append("# HELP openapi_worker_upstream_results_total Upstream business responses by status class and outcome.")
        lines.append("# TYPE openapi_worker_upstream_results_total counter")
        for (status_class, outcome) in sorted(self._upstream_results):
            lines.append(f'openapi_worker_upstream_results_total{{status_class="{status_class}",outcome="{outcome}"}} {self._upstream_results[(status_class, outcome)]}')

        lines.append("# HELP openapi_worker_errors_total Worker errors by route and bounded error code.")
        lines.append("# TYPE openapi_worker_errors_total counter")
        for (route, code) in sorted(self._route_errors):
            lines.append(f'openapi_worker_errors_total{{route="{route}",code="{code}"}} {self._route_errors[(route, code)]}')

        lines.append("# HELP openapi_worker_requests_total Internal route requests by outcome.")
        lines.append("# TYPE openapi_worker_requests_total counter")
        for (route, outcome) in sorted(self._route_requests):
            lines.append(f'openapi_worker_requests_total{{route="{route}",outcome="{outcome}"}} {self._route_requests[(route, outcome)]}')

        lines.append("# HELP openapi_worker_request_seconds Internal route handling latency.")
        lines.append("# TYPE openapi_worker_request_seconds histogram")
        lines.extend(self._route_durations.render())
        lines.append("# HELP openapi_worker_compilation_seconds Cold snapshot compilation latency.")
        lines.append("# TYPE openapi_worker_compilation_seconds histogram")
        lines.extend(self._compile_durations.render())
        return "\n".join(lines) + "\n"


_KNOWN_ERROR_CODES = {
    "protocol_version_unsupported", "invalid_request", "request_too_large", "snapshot_required",
    "snapshot_invalid", "snapshot_recovery_failed", "snapshot_too_large", "snapshot_cache_full",
    "compile_busy", "compile_timeout", "lease_invalid", "business_authorization_required",
    "operation_not_found", "transport_uncertain", "upstream_protocol_unsupported", "response_limit_exceeded",
    "response_too_large", "worker_unavailable", "internal_error",
}


def error_code(err: WorkerError) -> str:
    return err.code if err.code in _KNOWN_ERROR_CODES else _UNSPECIFIED


def record_executor_error(metrics: "WorkerMetrics", err: WorkerError) -> None:
    """Classify executor-raised errors into the fixed snapshot-event vocabulary."""
    if err.code in ("snapshot_required", "snapshot_recovery_failed"):
        metrics.record_snapshot_event(err.code)
    elif err.code in ("snapshot_invalid", "snapshot_too_large", "snapshot_cache_full", "compile_busy", "compile_timeout"):
        metrics.record_snapshot_event("compile_rejected")
    else:
        metrics.record_snapshot_event("execute_rejected")
