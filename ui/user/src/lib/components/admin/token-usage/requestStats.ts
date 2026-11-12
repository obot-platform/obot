import { m } from '$lib/i18n';

export function formatCount(value: number) {
	return Intl.NumberFormat().format(Math.round(value));
}

export function formatLatency(ms: number) {
	return m.audit_usage_usage_latency_ms({ ms: formatCount(ms) });
}

export function successRate(totals: { requests: number; successes: number }) {
	return totals.requests ? (totals.successes / totals.requests) * 100 : 0;
}
