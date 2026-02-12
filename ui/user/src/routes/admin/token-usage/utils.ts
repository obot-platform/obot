import { lightenHex } from '$lib/colors';
import type { OrgUser, TokenUsage } from '$lib/services/admin/types';
import { getUserDisplayName } from '$lib/utils';
import {
	addMinutes,
	addHours,
	addDays,
	addWeeks,
	addMonths,
	differenceInCalendarDays,
	differenceInHours,
	differenceInMinutes,
	format,
	startOfMinute,
	startOfHour,
	startOfDay,
	startOfWeek,
	startOfMonth
} from 'date-fns';

type BucketKind =
	| 'minute'
	| '5min'
	| '10min'
	| 'hour'
	| '2hour'
	| '4hour'
	| 'day'
	| 'week'
	| 'month';

const MAX_MINUTE_BUCKETS = 24;
const MAX_HOUR_BUCKETS = 24;
type TokenTotals = { prompt: number; completion: number };

const PROMPT_SUFFIX = '_input_tokens';
const COMPLETION_SUFFIX = '_output_tokens';

export function getBucketKind(rangeStart: Date, rangeEnd: Date): BucketKind {
	const hours = differenceInHours(rangeEnd, rangeStart);
	if (hours <= 2) {
		const totalMinutes = differenceInMinutes(rangeEnd, rangeStart) + 1;
		if (totalMinutes > MAX_MINUTE_BUCKETS) {
			const fiveMinBuckets = Math.ceil(totalMinutes / 5);
			return fiveMinBuckets > MAX_MINUTE_BUCKETS ? '10min' : '5min';
		}
		return 'minute';
	}
	if (hours <= 48) {
		const totalHours = hours + 1;
		if (totalHours > MAX_HOUR_BUCKETS) {
			const twoHourBuckets = Math.ceil(totalHours / 2);
			return twoHourBuckets > MAX_HOUR_BUCKETS ? '4hour' : '2hour';
		}
		return 'hour';
	}
	const days = differenceInCalendarDays(rangeEnd, rangeStart) + 1;
	if (days <= 14) return 'day';
	if (days <= 84) return 'week';
	return 'month';
}

function startOfFiveMinutes(date: Date): Date {
	const d = startOfMinute(new Date(date));
	const m = d.getMinutes();
	d.setMinutes(Math.floor(m / 5) * 5, 0, 0);
	return d;
}

function startOfTenMinutes(date: Date): Date {
	const d = startOfMinute(new Date(date));
	const m = d.getMinutes();
	d.setMinutes(Math.floor(m / 10) * 10, 0, 0);
	return d;
}

function startOfTwoHours(date: Date): Date {
	const d = startOfHour(new Date(date));
	const h = d.getHours();
	d.setHours(Math.floor(h / 2) * 2, 0, 0, 0);
	return d;
}

function startOfFourHours(date: Date): Date {
	const d = startOfHour(new Date(date));
	const h = d.getHours();
	d.setHours(Math.floor(h / 4) * 4, 0, 0, 0);
	return d;
}

export function getBucketStart(date: Date, kind: BucketKind): Date {
	const d = new Date(date);
	if (kind === 'minute') return startOfMinute(d);
	if (kind === '5min') return startOfFiveMinutes(d);
	if (kind === '10min') return startOfTenMinutes(d);
	if (kind === 'hour') return startOfHour(d);
	if (kind === '2hour') return startOfTwoHours(d);
	if (kind === '4hour') return startOfFourHours(d);
	if (kind === 'day') return startOfDay(d);
	if (kind === 'week') return startOfWeek(d, { weekStartsOn: 1 });
	return startOfMonth(d);
}

function formatMinuteLikeLabel(bucketStart: Date): string {
	const d = new Date(bucketStart);
	if (d.getHours() === 0 && d.getMinutes() === 0) {
		return format(d, 'MMM d');
	}
	if (d.getMinutes() === 0) return format(d, 'ha').toLowerCase();
	return format(d, 'h:mma').toLowerCase();
}

export function formatBucketLabel(bucketStart: Date, kind: BucketKind): string {
	if (kind === 'minute' || kind === '5min' || kind === '10min') {
		return formatMinuteLikeLabel(bucketStart);
	}
	if (kind === 'hour' || kind === '2hour' || kind === '4hour') {
		const d = new Date(bucketStart);
		if (d.getHours() === 0 && d.getMinutes() === 0) {
			return format(d, 'MMM d');
		}
		return format(d, 'ha').toLowerCase();
	}
	if (kind === 'day') return format(bucketStart, 'MMM d');
	if (kind === 'week') return `${format(bucketStart, 'MMM d')} –`;
	return format(bucketStart, 'MMM yyyy');
}

export type BucketInRange = { bucketKey: string; label: string };

/** Returns every bucket (key + label) that falls within [rangeStart, rangeEnd] for the given kind. */
export function getBucketsInRange(
	rangeStart: Date,
	rangeEnd: Date,
	kind: BucketKind
): BucketInRange[] {
	const buckets: BucketInRange[] = [];
	let cursor = getBucketStart(new Date(rangeStart), kind);
	const endBucket = getBucketStart(new Date(rangeEnd), kind);
	let addOne: (d: Date) => Date;
	switch (kind) {
		case 'minute':
			addOne = (d: Date) => addMinutes(d, 1);
			break;
		case '5min':
			addOne = (d: Date) => addMinutes(d, 5);
			break;
		case '10min':
			addOne = (d: Date) => addMinutes(d, 10);
			break;
		case 'hour':
			addOne = (d: Date) => addHours(d, 1);
			break;
		case '2hour':
			addOne = (d: Date) => addHours(d, 2);
			break;
		case '4hour':
			addOne = (d: Date) => addHours(d, 4);
			break;
		case 'day':
			addOne = (d: Date) => addDays(d, 1);
			break;
		case 'week':
			addOne = (d: Date) => addWeeks(d, 1);
			break;
		case 'month':
			addOne = (d: Date) => addMonths(d, 1);
			break;
		default:
			addOne = (d: Date) => addMonths(d, 1);
	}
	while (cursor <= endBucket) {
		buckets.push({
			bucketKey: cursor.toISOString(),
			label: formatBucketLabel(cursor, kind)
		});
		cursor = addOne(cursor);
	}
	return buckets;
}

export function getUserLabels(
	users: Map<string, OrgUser>,
	userKeys: string[]
): Map<string, string> {
	const simpleLabels = new Map(userKeys.map((k) => [k, getUserDisplayName(users, k)]));
	const displayCounts = new Map<string, number>();
	for (const label of simpleLabels.values()) {
		displayCounts.set(label, (displayCounts.get(label) ?? 0) + 1);
	}
	return new Map(
		userKeys.map((k) => {
			const simple = simpleLabels.get(k)!;
			const label =
				(displayCounts.get(simple) ?? 0) > 1 ? getUserDisplayName(users, k, () => true) : simple;
			return [k, label];
		})
	);
}

export function aggregateByBucketDefault(
	data: TokenUsage[],
	getBucketKey: (row: TokenUsage) => string,
	getBucketLabel: (bucketKey: string) => string
): { bucket: string; input_tokens: number; output_tokens: number }[] {
	const bucketToTokens = new Map<string, TokenTotals>();
	for (const row of data) {
		const bucketKey = getBucketKey(row);
		let totals = bucketToTokens.get(bucketKey);
		if (!totals) {
			totals = { prompt: 0, completion: 0 };
			bucketToTokens.set(bucketKey, totals);
		}
		totals.prompt += row.promptTokens ?? 0;
		totals.completion += row.completionTokens ?? 0;
	}
	const sortedBuckets = [...bucketToTokens.keys()].sort();
	return sortedBuckets.map((bucketKey) => {
		const totals = bucketToTokens.get(bucketKey)!;
		return {
			bucket: getBucketLabel(bucketKey),
			input_tokens: totals.prompt,
			output_tokens: totals.completion
		};
	});
}

/** Like aggregateByBucketDefault but returns one row per bucket in [rangeStart, rangeEnd], with zeros for buckets that have no data. */
export function aggregateByBucketDefaultInRange(
	data: TokenUsage[],
	rangeStart: Date,
	rangeEnd: Date
): { bucket: string; input_tokens: number; output_tokens: number }[] {
	const kind = getBucketKind(rangeStart, rangeEnd);
	const bucketsInRange = getBucketsInRange(rangeStart, rangeEnd, kind);
	const bucketToTokens = new Map<string, TokenTotals>();
	for (const row of data) {
		const bucketKey = getBucketStart(new Date(row.date), kind).toISOString();
		let totals = bucketToTokens.get(bucketKey);
		if (!totals) {
			totals = { prompt: 0, completion: 0 };
			bucketToTokens.set(bucketKey, totals);
		}
		totals.prompt += row.promptTokens ?? 0;
		totals.completion += row.completionTokens ?? 0;
	}
	return bucketsInRange.map(({ bucketKey, label }) => {
		const totals = bucketToTokens.get(bucketKey) ?? { prompt: 0, completion: 0 };
		return {
			bucket: label,
			input_tokens: totals.prompt,
			output_tokens: totals.completion
		};
	});
}

export function aggregateByBucketGrouped(
	data: TokenUsage[],
	getBucketKey: (row: TokenUsage) => string,
	getBucketLabel: (bucketKey: string) => string,
	getGroupKey: (row: TokenUsage) => string,
	keyToDisplayLabel: (key: string) => string
): Record<string, string | number>[] {
	const bucketToGroups = new Map<string, Record<string, TokenTotals>>();
	const allGroupKeys = new Set<string>();
	for (const row of data) {
		const bucketKey = getBucketKey(row);
		let totals = bucketToGroups.get(bucketKey);
		if (!totals) {
			totals = {};
			bucketToGroups.set(bucketKey, totals);
		}
		const groupKey = getGroupKey(row);
		allGroupKeys.add(groupKey);
		const current = totals[groupKey] ?? { prompt: 0, completion: 0 };
		current.prompt += row.promptTokens ?? 0;
		current.completion += row.completionTokens ?? 0;
		totals[groupKey] = current;
	}
	const sortedBuckets = [...bucketToGroups.keys()].sort();
	const groupKeys = [...allGroupKeys].sort();
	return sortedBuckets.map((bucketKey) => {
		const totals = bucketToGroups.get(bucketKey)!;
		const label = getBucketLabel(bucketKey);
		const row: Record<string, string | number> = { bucket: label };
		for (const k of groupKeys) {
			const displayLabel = keyToDisplayLabel(k);
			const groupTotals = totals[k] ?? { prompt: 0, completion: 0 };
			row[`${displayLabel}_input_tokens`] = groupTotals.prompt;
			row[`${displayLabel}_output_tokens`] = groupTotals.completion;
		}
		return row;
	});
}

/** Like aggregateByBucketGrouped but returns one row per bucket in [rangeStart, rangeEnd], with zeros for buckets that have no data. */
export function aggregateByBucketGroupedInRange(
	data: TokenUsage[],
	rangeStart: Date,
	rangeEnd: Date,
	getGroupKey: (row: TokenUsage) => string,
	keyToDisplayLabel: (key: string) => string
): Record<string, string | number>[] {
	const kind = getBucketKind(rangeStart, rangeEnd);
	const bucketsInRange = getBucketsInRange(rangeStart, rangeEnd, kind);
	const bucketToGroups = new Map<string, Record<string, TokenTotals>>();
	const allGroupKeys = new Set<string>();
	for (const row of data) {
		const bucketKey = getBucketStart(new Date(row.date), kind).toISOString();
		let totals = bucketToGroups.get(bucketKey);
		if (!totals) {
			totals = {};
			bucketToGroups.set(bucketKey, totals);
		}
		const groupKey = getGroupKey(row);
		allGroupKeys.add(groupKey);
		const current = totals[groupKey] ?? { prompt: 0, completion: 0 };
		current.prompt += row.promptTokens ?? 0;
		current.completion += row.completionTokens ?? 0;
		totals[groupKey] = current;
	}
	const groupKeys = [...allGroupKeys].sort();
	return bucketsInRange.map(({ bucketKey, label }) => {
		const totals = bucketToGroups.get(bucketKey) ?? {};
		const row: Record<string, string | number> = { bucket: label };
		for (const k of groupKeys) {
			const displayLabel = keyToDisplayLabel(k);
			const groupTotals = totals[k] ?? { prompt: 0, completion: 0 };
			row[`${displayLabel}_input_tokens`] = groupTotals.prompt;
			row[`${displayLabel}_output_tokens`] = groupTotals.completion;
		}
		return row;
	});
}

export function buildStackedSeriesColors(
	rows: Record<string, unknown>[],
	palette: string[],
	fallbackColor: string
): { key: string; color: string }[] {
	if (!rows.length) return [];
	const keys = new Set<string>();
	for (const row of rows) {
		for (const key of Object.keys(row)) {
			if (key !== 'bucket') keys.add(key);
		}
	}
	const sortedKeys = [...keys].sort();
	const labels = new Set<string>();
	for (const key of sortedKeys) {
		if (key.endsWith(PROMPT_SUFFIX)) labels.add(key.slice(0, -PROMPT_SUFFIX.length));
		else if (key.endsWith(COMPLETION_SUFFIX)) labels.add(key.slice(0, -COMPLETION_SUFFIX.length));
	}
	const sortedLabels = [...labels].sort();
	const labelIndex = new Map(sortedLabels.map((label, i) => [label, i]));
	return sortedKeys.map((key) => {
		const isPrompt = key.endsWith(PROMPT_SUFFIX);
		const label = isPrompt
			? key.slice(0, -PROMPT_SUFFIX.length)
			: key.slice(0, -COMPLETION_SUFFIX.length);
		const idx = labelIndex.get(label) ?? 0;
		const baseColor = idx < palette.length ? palette[idx]! : fallbackColor;
		const color = isPrompt ? baseColor : lightenHex(baseColor, 0.5);
		return { key, color };
	});
}
