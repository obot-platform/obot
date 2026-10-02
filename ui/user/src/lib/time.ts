import { getLocale, m } from '$lib/i18n';

export type TimeDisplayFormat = '12h' | '24h';

export function formatTime(time: Date | string, format: TimeDisplayFormat) {
	const now = new Date();
	if (typeof time === 'string') {
		time = new Date(time);
	}
	const hour12 = format === '12h';
	if (
		time.getDate() == now.getDate() &&
		time.getMonth() == now.getMonth() &&
		time.getFullYear() == now.getFullYear()
	) {
		return time.toLocaleTimeString(getLocale(), {
			hour: 'numeric',
			minute: 'numeric',
			hour12
		});
	}
	return time
		.toLocaleString(getLocale(), {
			year: 'numeric',
			month: '2-digit',
			day: '2-digit',
			hour: 'numeric',
			minute: '2-digit',
			hour12
		})
		.replace(/\//g, '-')
		.replace(/,/g, '');
}

export interface TimeAgoResult {
	relativeTime: string;
	fullDate: string;
}

/**
 * Formats a timestamp into a relative time description ("2 hours ago") and a localized full date string
 * @param timestamp ISO string date or undefined
 * @param granularity return a relative time with this granularity
 * @returns Object containing relativeTime and fullDate strings
 */
export function formatTimeAgo(timestamp: string | undefined, granularity?: string): TimeAgoResult {
	if (!timestamp) return { relativeTime: '', fullDate: '' };

	const now = new Date();
	const date = new Date(timestamp);
	if (isNaN(date.getTime())) return { relativeTime: '', fullDate: '' };

	const seconds = Math.floor((now.getTime() - date.getTime()) / 1000);

	// Format the full date for the tooltip
	const options: Intl.DateTimeFormatOptions = {
		weekday: 'long',
		year: 'numeric',
		month: 'long',
		day: 'numeric',
		hour: '2-digit',
		minute: '2-digit',
		hour12: true
	};
	const fullDate = date.toLocaleString(getLocale(), options);

	// Relative time calculation
	let relativeTime: string;
	let interval = Math.floor(seconds / 31536000);
	if (interval >= 1) {
		relativeTime =
			interval === 1
				? m.core_time_years_ago_one()
				: m.core_time_years_ago_other({ count: interval });
	} else if (granularity === 'year') {
		relativeTime = m.core_time_this_year();
	} else {
		interval = Math.floor(seconds / 2592000);
		if (interval >= 1) {
			relativeTime =
				interval === 1
					? m.core_time_months_ago_one()
					: m.core_time_months_ago_other({ count: interval });
		} else if (granularity === 'month') {
			relativeTime = m.core_time_this_month();
		} else {
			interval = Math.floor(seconds / 86400);
			if (interval >= 1) {
				relativeTime =
					interval === 1
						? m.core_time_days_ago_one()
						: m.core_time_days_ago_other({ count: interval });
			} else if (granularity === 'day') {
				relativeTime = m.core_time_today();
			} else {
				interval = Math.floor(seconds / 3600);
				if (interval >= 1) {
					relativeTime =
						interval === 1
							? m.core_time_hours_ago_one()
							: m.core_time_hours_ago_other({ count: interval });
				} else if (granularity === 'hour') {
					relativeTime = m.core_time_in_last_hour();
				} else {
					interval = Math.floor(seconds / 60);
					if (interval >= 1) {
						relativeTime =
							interval === 1
								? m.core_time_minutes_ago_one()
								: m.core_time_minutes_ago_other({ count: interval });
					} else if (granularity === 'minute') {
						relativeTime = m.core_time_in_last_minute();
					} else {
						if (seconds < 10) return { relativeTime: m.core_time_just_now(), fullDate };
						relativeTime = m.core_time_seconds_ago({ count: Math.floor(seconds) });
					}
				}
			}
		}
	}

	return { relativeTime, fullDate };
}

/**
 * Formats a future timestamp into a relative time description ("in 2 hours") and a localized full date string
 * @param timestamp ISO string date or undefined
 * @returns Object containing relativeTime and fullDate strings
 */
export function formatTimeUntil(timestamp: string | undefined): TimeAgoResult {
	if (!timestamp) return { relativeTime: '', fullDate: '' };

	const now = new Date();
	const date = new Date(timestamp);
	if (isNaN(date.getTime())) return { relativeTime: '', fullDate: '' };

	const seconds = Math.floor((date.getTime() - now.getTime()) / 1000);

	// Format the full date for the tooltip
	const options: Intl.DateTimeFormatOptions = {
		weekday: 'long',
		year: 'numeric',
		month: 'long',
		day: 'numeric',
		hour: '2-digit',
		minute: '2-digit',
		hour12: true
	};
	const fullDate = date.toLocaleString(getLocale(), options);

	// If the date is in the past, return "Expired"
	if (seconds < 0) {
		return { relativeTime: m.core_time_expired(), fullDate };
	}

	// Relative time calculation for future dates
	let relativeTime: string;
	let interval = Math.floor(seconds / 31536000);
	if (interval >= 1) {
		relativeTime =
			interval === 1 ? m.core_time_in_years_one() : m.core_time_in_years_other({ count: interval });
	} else {
		interval = Math.floor(seconds / 2592000);
		if (interval >= 1) {
			relativeTime =
				interval === 1
					? m.core_time_in_months_one()
					: m.core_time_in_months_other({ count: interval });
		} else {
			interval = Math.floor(seconds / 86400);
			if (interval >= 1) {
				relativeTime =
					interval === 1
						? m.core_time_in_days_one()
						: m.core_time_in_days_other({ count: interval });
			} else {
				interval = Math.floor(seconds / 3600);
				if (interval >= 1) {
					relativeTime =
						interval === 1
							? m.core_time_in_hours_one()
							: m.core_time_in_hours_other({ count: interval });
				} else {
					interval = Math.floor(seconds / 60);
					if (interval >= 1) {
						relativeTime =
							interval === 1
								? m.core_time_in_minutes_one()
								: m.core_time_in_minutes_other({ count: interval });
					} else {
						relativeTime = m.core_time_in_less_than_minute();
					}
				}
			}
		}
	}

	return { relativeTime, fullDate };
}

export function formatTimeRange(startTime: Date | string, endTime: Date | string): string {
	if (startTime == null || endTime == null) return '';

	const start = startTime instanceof Date ? startTime : new Date(startTime);
	const end = endTime instanceof Date ? endTime : new Date(endTime);
	const now = new Date();

	const durationInHours = (end.getTime() - start.getTime()) / (1000 * 60 * 60);
	const endIsCloseToNow = Math.abs(end.getTime() - now.getTime()) < 2 * 60 * 1000;

	// Preset ranges ending close to now (order matters: check specific durations first)
	if (Math.abs(durationInHours - 1) < 0.02 && endIsCloseToNow) return m.core_time_last_hour();
	if (Math.abs(durationInHours - 6) < 0.02 && endIsCloseToNow) return m.core_time_last_6_hours();
	if (Math.abs(durationInHours - 24) < 0.1 && endIsCloseToNow) return m.core_time_last_24_hours();

	// "Last X Days" presets: start = midnight N days ago (local), end = now (local). When stored
	// as UTC, duration becomes N*24 + (hours since midnight local), so we see N*24..N*24+24.
	const endWithinDay = Math.abs(end.getTime() - now.getTime()) < 24 * 60 * 60 * 1000;

	// Last 7 Days: 144h (6d) to 193h (7d+1d timezone/end-of-day slack)
	if (endWithinDay && durationInHours >= 144 && durationInHours < 193)
		return m.core_time_last_7_days();
	// Last 30 Days: 696h (29d) to 745h
	if (endWithinDay && durationInHours >= 696 && durationInHours < 745)
		return m.core_time_last_30_days();
	// Last 60 Days: 1416h (59d) to 1465h
	if (endWithinDay && durationInHours >= 1416 && durationInHours < 1465)
		return m.core_time_last_60_days();
	// Last 90 Days: 2136h (89d) to 2185h
	if (endWithinDay && durationInHours >= 2136 && durationInHours < 2185)
		return m.core_time_last_90_days();

	// Check if it's a whole day (start at 00:00 and end at 23:59 or next day 00:00)
	const startHour = start.getHours();
	const startMinute = start.getMinutes();
	const endHour = end.getHours();
	const endMinute = end.getMinutes();

	// Check if start and end are on the same date
	const isSameDate =
		start.getDate() === end.getDate() &&
		start.getMonth() === end.getMonth() &&
		start.getFullYear() === end.getFullYear();

	const isWholeDay =
		isSameDate &&
		startHour === 0 &&
		startMinute === 0 &&
		((endHour === 0 && endMinute === 0) || (endHour === 23 && endMinute === 59));

	if (isWholeDay) {
		// Format as just the date
		return start.toLocaleDateString(getLocale(), {
			month: 'short',
			day: 'numeric',
			year: 'numeric'
		});
	}

	// Check if both times are at midnight (00:00)
	const bothAtMidnight = startHour === 0 && startMinute === 0 && endHour === 0 && endMinute === 0;

	if (bothAtMidnight) {
		// Format as just date range when both times are at midnight
		const startDateFormatted = start.toLocaleDateString(getLocale(), {
			month: 'short',
			day: 'numeric',
			year: 'numeric'
		});

		const endDateFormatted = end.toLocaleDateString(getLocale(), {
			month: 'short',
			day: 'numeric',
			year: 'numeric'
		});

		return `${startDateFormatted} - ${endDateFormatted}`;
	}

	// Format as date & time range
	const startFormatted = start.toLocaleString(getLocale(), {
		month: 'numeric',
		day: 'numeric',
		year: '2-digit',
		hour: 'numeric',
		minute: '2-digit',
		hour12: true
	});

	const endFormatted = end.toLocaleString(getLocale(), {
		month: 'numeric',
		day: 'numeric',
		year: '2-digit',
		hour: 'numeric',
		minute: '2-digit',
		hour12: true
	});

	return `${startFormatted} - ${endFormatted}`;
}

export function getTimeRangeShorthand(startTime: Date | string, endTime: Date | string): string {
	const start = startTime instanceof Date ? startTime : new Date(startTime);
	const end = endTime instanceof Date ? endTime : new Date(endTime);
	const diffMs = end.getTime() - start.getTime();

	const hours = diffMs / (1000 * 60 * 60);
	const days = hours / 24;
	const weeks = days / 7;
	const months = days / 30.44; // Average days per month
	const years = days / 365.25; // Average days per year

	if (years >= 1) {
		return `${Math.round(years)}y`;
	} else if (months >= 1) {
		return `${Math.round(months)}mo`;
	} else if (weeks >= 1) {
		return `${Math.round(weeks)}w`;
	} else if (days >= 1) {
		return `${Math.round(days)}d`;
	} else {
		return `${Math.round(hours)}h`;
	}
}

export function formatLogTimestamp(time: Date | string, format: TimeDisplayFormat) {
	return new Date(time)
		.toLocaleString(getLocale(), {
			year: 'numeric',
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit',
			hour12: format === '12h',
			timeZoneName: 'short'
		})
		.replace(/,/g, '');
}

const auditLogTableTimestampFormatter = new Intl.DateTimeFormat('en-US', {
	year: 'numeric',
	month: '2-digit',
	day: '2-digit',
	hour: '2-digit',
	minute: '2-digit',
	second: '2-digit',
	hourCycle: 'h23',
	timeZoneName: 'short'
});

export function formatAuditLogTableTimestamp(time: Date | string) {
	const parts = auditLogTableTimestampFormatter.formatToParts(new Date(time));
	const value = (type: Intl.DateTimeFormatPartTypes) =>
		parts.find((part) => part.type === type)?.value ?? '';

	return `${value('year')}-${value('month')}-${value('day')} ${value('hour')}:${value('minute')}:${value('second')} ${value('timeZoneName')}`;
}

export function isRecent(created: string, withinMinutes = 1): boolean {
	const diff = Date.now() - new Date(created).getTime();
	return diff < withinMinutes * 60 * 1000;
}

/** Localized AM/PM labels for 12-hour time pickers (e.g. 午前/午後, 오전/오후, 上午/下午). */
export function getDayPeriodLabels(): { am: string; pm: string } {
	const format = new Intl.DateTimeFormat(getLocale(), {
		hour: 'numeric',
		hour12: true,
		timeZone: 'UTC'
	});
	const label = (hour: number) =>
		format.formatToParts(new Date(Date.UTC(2024, 0, 1, hour))).find((p) => p.type === 'dayPeriod')
			?.value ?? (hour < 12 ? 'AM' : 'PM');
	return { am: label(9), pm: label(21) };
}
