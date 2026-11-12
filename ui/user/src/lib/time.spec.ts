import { getTimeRangeFilter } from '$lib/time';
import { set, subDays } from 'date-fns';
import { describe, expect, it } from 'vitest';

describe('getTimeRangeFilter', () => {
	it('returns explicit date overrides unchanged', () => {
		const startTime = new Date('2024-01-01T00:00:00.000Z');
		const endTime = new Date('2024-01-08T00:00:00.000Z');

		expect(getTimeRangeFilter({ startTime, endTime })).toEqual({ startTime, endTime });
	});

	it('zeros start seconds and sets the end to 59 seconds', () => {
		const range = getTimeRangeFilter({
			start_time: '2024-03-01T15:45:30.123Z',
			end_time: '2024-03-10T08:12:45.999Z'
		});

		expect(range.startTime.getSeconds()).toBe(0);
		expect(range.startTime.getMilliseconds()).toBe(0);
		expect(range.endTime.getSeconds()).toBe(59);
		expect(range.endTime.getMilliseconds()).toBe(0);
		expect(range.startTime.getTime()).toBeLessThan(range.endTime.getTime());
	});

	it('falls back to 7 days before the end when the start is missing or after the end', () => {
		const end_time = '2024-03-10T12:00:00.000Z';
		const endTime = set(new Date(end_time), { milliseconds: 0, seconds: 59 });
		const startTime = set(subDays(endTime, 7), { seconds: 0, milliseconds: 0 });

		expect(getTimeRangeFilter({ end_time })).toEqual({ startTime, endTime });
		expect(
			getTimeRangeFilter({
				start_time: '2024-03-20T00:00:00.000Z',
				end_time
			})
		).toEqual({ startTime, endTime });
	});

	it('uses the requested day count for the default start', () => {
		const end_time = '2024-03-10T12:00:00.000Z';
		const endTime = set(new Date(end_time), { milliseconds: 0, seconds: 59 });
		const startTime = set(subDays(endTime, 30), { seconds: 0, milliseconds: 0 });

		expect(getTimeRangeFilter({ end_time, days: 30 })).toEqual({ startTime, endTime });
	});
});
