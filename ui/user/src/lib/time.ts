export function formatTime(time: Date | string) {
	const now = new Date();
	if (typeof time === 'string') {
		time = new Date(time);
	}
	if (
		time.getDate() == now.getDate() &&
		time.getMonth() == now.getMonth() &&
		time.getFullYear() == now.getFullYear()
	) {
		return time.toLocaleTimeString(undefined, {
			hour: 'numeric',
			minute: 'numeric'
		});
	}
	return time
		.toLocaleString(undefined, {
			year: 'numeric',
			month: '2-digit',
			day: '2-digit',
			hour: 'numeric',
			minute: '2-digit',
			hour12: true
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
	const fullDate = date.toLocaleString(undefined, options);

	// Relative time calculation
	let relativeTime = '';
	let interval = Math.floor(seconds / 31536000);
	if (interval >= 1) {
		relativeTime = interval === 1 ? '1 year ago' : `${interval} years ago`;
	} else if (granularity === 'year') {
		relativeTime = 'This year';
	} else {
		interval = Math.floor(seconds / 2592000);
		if (interval >= 1) {
			relativeTime = interval === 1 ? '1 month ago' : `${interval} months ago`;
		} else if (granularity === 'month') {
			relativeTime = 'This month';
		} else {
			interval = Math.floor(seconds / 86400);
			if (interval >= 1) {
				relativeTime = interval === 1 ? '1 day ago' : `${interval} days ago`;
			} else if (granularity === 'day') {
				relativeTime = 'Today';
			} else {
				interval = Math.floor(seconds / 3600);
				if (interval >= 1) {
					relativeTime = interval === 1 ? '1 hour ago' : `${interval} hours ago`;
				} else if (granularity === 'hour') {
					relativeTime = 'In the last hour';
				} else {
					interval = Math.floor(seconds / 60);
					if (interval >= 1) {
						relativeTime = interval === 1 ? '1 minute ago' : `${interval} minutes ago`;
					} else if (granularity === 'minute') {
						relativeTime = 'In the last minute';
					} else {
						if (seconds < 10) return { relativeTime: 'just now', fullDate };
						relativeTime = `${Math.floor(seconds)} seconds ago`;
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
	const fullDate = date.toLocaleString(undefined, options);

	// If the date is in the past, return "Expired"
	if (seconds < 0) {
		return { relativeTime: 'Expired', fullDate };
	}

	// Relative time calculation for future dates
	let relativeTime = '';
	let interval = Math.floor(seconds / 31536000);
	if (interval >= 1) {
		relativeTime = interval === 1 ? 'in 1 year' : `in ${interval} years`;
	} else {
		interval = Math.floor(seconds / 2592000);
		if (interval >= 1) {
			relativeTime = interval === 1 ? 'in 1 month' : `in ${interval} months`;
		} else {
			interval = Math.floor(seconds / 86400);
			if (interval >= 1) {
				relativeTime = interval === 1 ? 'in 1 day' : `in ${interval} days`;
			} else {
				interval = Math.floor(seconds / 3600);
				if (interval >= 1) {
					relativeTime = interval === 1 ? 'in 1 hour' : `in ${interval} hours`;
				} else {
					interval = Math.floor(seconds / 60);
					if (interval >= 1) {
						relativeTime = interval === 1 ? 'in 1 minute' : `in ${interval} minutes`;
					} else {
						relativeTime = 'in less than a minute';
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
	if (Math.abs(durationInHours - 1) < 0.02 && endIsCloseToNow) return 'Last Hour';
	if (Math.abs(durationInHours - 6) < 0.02 && endIsCloseToNow) return 'Last 6 Hours';
	if (Math.abs(durationInHours - 24) < 0.1 && endIsCloseToNow) return 'Last 24 Hours';

	// "Last X Days" presets: start = midnight N days ago (local), end = now (local). When stored
	// as UTC, duration becomes N*24 + (hours since midnight local), so we see N*24..N*24+24.
	const endWithinDay = Math.abs(end.getTime() - now.getTime()) < 24 * 60 * 60 * 1000;

	// Last 7 Days: 144h (6d) to 193h (7d+1d timezone/end-of-day slack)
	if (endWithinDay && durationInHours >= 144 && durationInHours < 193) return 'Last 7 Days';
	// Last 30 Days: 696h (29d) to 745h
	if (endWithinDay && durationInHours >= 696 && durationInHours < 745) return 'Last 30 Days';
	// Last 60 Days: 1416h (59d) to 1465h
	if (endWithinDay && durationInHours >= 1416 && durationInHours < 1465) return 'Last 60 Days';
	// Last 90 Days: 2136h (89d) to 2185h
	if (endWithinDay && durationInHours >= 2136 && durationInHours < 2185) return 'Last 90 Days';

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
		return start.toLocaleDateString(undefined, {
			month: 'short',
			day: 'numeric',
			year: 'numeric'
		});
	}

	// Check if both times are at midnight (00:00)
	const bothAtMidnight = startHour === 0 && startMinute === 0 && endHour === 0 && endMinute === 0;

	if (bothAtMidnight) {
		// Format as just date range when both times are at midnight
		const startDateFormatted = start.toLocaleDateString(undefined, {
			month: 'short',
			day: 'numeric',
			year: 'numeric'
		});

		const endDateFormatted = end.toLocaleDateString(undefined, {
			month: 'short',
			day: 'numeric',
			year: 'numeric'
		});

		return `${startDateFormatted} - ${endDateFormatted}`;
	}

	// Format as date & time range
	const startFormatted = start.toLocaleString(undefined, {
		month: 'numeric',
		day: 'numeric',
		year: '2-digit',
		hour: 'numeric',
		minute: '2-digit',
		hour12: true
	});

	const endFormatted = end.toLocaleString(undefined, {
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
