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
 * @returns Object containing relativeTime and fullDate strings
 */
export function formatTimeAgo(timestamp: string | undefined): TimeAgoResult {
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
	} else {
		interval = Math.floor(seconds / 2592000);
		if (interval >= 1) {
			relativeTime = interval === 1 ? '1 month ago' : `${interval} months ago`;
		} else {
			interval = Math.floor(seconds / 86400);
			if (interval >= 1) {
				relativeTime = interval === 1 ? '1 day ago' : `${interval} days ago`;
			} else {
				interval = Math.floor(seconds / 3600);
				if (interval >= 1) {
					relativeTime = interval === 1 ? '1 hour ago' : `${interval} hours ago`;
				} else {
					interval = Math.floor(seconds / 60);
					if (interval >= 1) {
						relativeTime = interval === 1 ? '1 minute ago' : `${interval} minutes ago`;
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

export function formatTimeRange(startTime: string, endTime: string): string {
	if (!startTime || !endTime) return '';

	const start = new Date(startTime);
	const end = new Date(endTime);
	const now = new Date();

	const durationInHours = (end.getTime() - start.getTime()) / (1000 * 60 * 60);
	const endIsCloseToNow = Math.abs(end.getTime() - now.getTime()) < 1000; // 1 sec leeway
	if (Math.abs(durationInHours - 24) < 0.1 && endIsCloseToNow) {
		// Within 6 minutes of exactly 24 hours and ending close to now
		return 'Last 24 Hours';
	}

	// Check if it's the last 7 days
	const sevenDayDurationInHours = (end.getTime() - start.getTime()) / (1000 * 60 * 60);
	const isLast7Days =
		Math.abs(sevenDayDurationInHours - 168) < 0.1 &&
		Math.abs(end.getTime() - now.getTime()) < 24 * 60 * 60 * 1000;

	if (isLast7Days) {
		return 'Last 7 Days';
	}

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

export function getTimeRangeShorthand(startTime: string, endTime: string): string {
	const start = new Date(startTime);
	const end = new Date(endTime);
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
