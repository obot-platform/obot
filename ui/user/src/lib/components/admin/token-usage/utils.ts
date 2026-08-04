import type { OrgUser } from '$lib/services';
import { getUserDisplayName } from '$lib/utils';
import {
	differenceInCalendarDays,
	differenceInHours,
	differenceInMinutes,
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

export type TimelineBucketRow = {
	date: string;
	category: string;
	inputTokens: number;
	outputTokens: number;
	totalTokens: number;
	cacheReadTokens: number;
	cacheWriteTokens: number;
	thinkingTokens: number;
	inputSpend: number;
	cacheReadSpend: number;
	cacheWriteSpend: number;
	outputSpend: number;
	totalSpend: number;
	bucketTokens: number;
	bucketSpend: number;
};

export function aggregateTimelineDataByBucket(
	data: {
		date: string | Date;
		category: string;
		inputTokens?: number;
		outputTokens?: number;
		totalTokens?: number;
		cacheReadTokens?: number;
		cacheWriteTokens?: number;
		thinkingTokens?: number;
		inputSpend?: number;
		cacheReadSpend?: number;
		cacheWriteSpend?: number;
		outputSpend?: number;
		totalSpend?: number;
		bucketTokens?: number;
		bucketSpend?: number;
	}[],
	rangeStart: Date,
	rangeEnd: Date
): TimelineBucketRow[] {
	if (data.length === 0) return [];
	const kind = getBucketKind(rangeStart, rangeEnd);
	const bucketToCategoryToTotals = new Map<
		string,
		Map<string, Omit<TimelineBucketRow, 'date' | 'category'>>
	>();
	for (const row of data) {
		const bucketKey = getBucketStart(new Date(row.date), kind).toISOString();
		let byCat = bucketToCategoryToTotals.get(bucketKey);
		if (!byCat) {
			byCat = new Map();
			bucketToCategoryToTotals.set(bucketKey, byCat);
		}
		const cat = row.category;
		const t = byCat.get(cat) ?? {
			inputTokens: 0,
			outputTokens: 0,
			totalTokens: 0,
			cacheReadTokens: 0,
			cacheWriteTokens: 0,
			thinkingTokens: 0,
			inputSpend: 0,
			cacheReadSpend: 0,
			cacheWriteSpend: 0,
			outputSpend: 0,
			totalSpend: 0,
			bucketTokens: 0,
			bucketSpend: 0
		};
		t.inputTokens += row.inputTokens ?? 0;
		t.outputTokens += row.outputTokens ?? 0;
		t.totalTokens += row.totalTokens ?? 0;
		t.cacheReadTokens += row.cacheReadTokens ?? 0;
		t.cacheWriteTokens += row.cacheWriteTokens ?? 0;
		t.thinkingTokens += row.thinkingTokens ?? 0;
		t.inputSpend += row.inputSpend ?? 0;
		t.cacheReadSpend += row.cacheReadSpend ?? 0;
		t.cacheWriteSpend += row.cacheWriteSpend ?? 0;
		t.outputSpend += row.outputSpend ?? 0;
		t.totalSpend += row.totalSpend ?? 0;
		t.bucketTokens += row.bucketTokens ?? 0;
		t.bucketSpend += row.bucketSpend ?? 0;
		byCat.set(cat, t);
	}
	const result: TimelineBucketRow[] = [];
	for (const [bucketKey, byCat] of bucketToCategoryToTotals) {
		for (const [category, totals] of byCat) {
			result.push({
				date: bucketKey,
				category,
				...totals
			});
		}
	}

	result.sort((a, b) => {
		const dateDiff = new Date(a.date).getTime() - new Date(b.date).getTime();
		if (dateDiff !== 0) return dateDiff;
		return a.category.localeCompare(b.category);
	});

	return result;
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
