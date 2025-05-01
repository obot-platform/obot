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
