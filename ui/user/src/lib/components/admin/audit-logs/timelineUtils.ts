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

function getBucketKind(rangeStart: Date, rangeEnd: Date): BucketKind {
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
	d.setMinutes(Math.floor(d.getMinutes() / 5) * 5, 0, 0);
	return d;
}

function startOfTenMinutes(date: Date): Date {
	const d = startOfMinute(new Date(date));
	d.setMinutes(Math.floor(d.getMinutes() / 10) * 10, 0, 0);
	return d;
}

function startOfTwoHours(date: Date): Date {
	const d = startOfHour(new Date(date));
	d.setHours(Math.floor(d.getHours() / 2) * 2, 0, 0, 0);
	return d;
}

function startOfFourHours(date: Date): Date {
	const d = startOfHour(new Date(date));
	d.setHours(Math.floor(d.getHours() / 4) * 4, 0, 0, 0);
	return d;
}

function getBucketStart(date: Date, kind: BucketKind): Date {
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

/** One row per bucket per category for StackedTimeline with primaryValueKey="count", secondaryValueKey="_secondary". */
export type AuditLogTimelineBucketRow = {
	createdAt: string;
	callType: string;
	count: number;
	_secondary: 0;
};

export function aggregateAuditLogsByBucket(
	logs: { createdAt: string; callType: string }[],
	rangeStart: Date,
	rangeEnd: Date
): AuditLogTimelineBucketRow[] {
	if (logs.length === 0) return [];
	const kind = getBucketKind(rangeStart, rangeEnd);
	const bucketToCategoryToCount = new Map<string, Map<string, number>>();
	for (const row of logs) {
		const bucketKey = getBucketStart(new Date(row.createdAt), kind).toISOString();
		let byCat = bucketToCategoryToCount.get(bucketKey);
		if (!byCat) {
			byCat = new Map();
			bucketToCategoryToCount.set(bucketKey, byCat);
		}
		const cat = row.callType || 'unknown';
		byCat.set(cat, (byCat.get(cat) ?? 0) + 1);
	}
	const result: AuditLogTimelineBucketRow[] = [];
	for (const [bucketKey, byCat] of bucketToCategoryToCount) {
		for (const [callType, count] of byCat) {
			result.push({
				createdAt: bucketKey,
				callType,
				count,
				_secondary: 0
			});
		}
	}

	result.sort((a, b) => {
		if (a.createdAt === b.createdAt) {
			return a.callType.localeCompare(b.callType);
		}
		return a.createdAt.localeCompare(b.createdAt);
	});

	return result;
}
