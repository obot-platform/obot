import { pluralize } from "~/lib/utils/pluralize";

export const timeSince = (date: Date) => {
	const seconds = Math.floor((new Date().getTime() - date.getTime()) / 1000);

	let interval = seconds / 31536000;

	if (interval > 1) {
		return Math.floor(interval) + " " + pluralize(interval, "year", "years");
	}
	interval = seconds / 2592000;
	if (interval > 1) {
		return Math.floor(interval) + " " + pluralize(interval, "month", "months");
	}
	interval = seconds / 86400;
	if (interval > 1) {
		return Math.floor(interval) + " " + pluralize(interval, "day", "days");
	}
	interval = seconds / 3600;
	if (interval > 1) {
		return Math.floor(interval) + " " + pluralize(interval, "hour", "hours");
	}
	interval = seconds / 60;
	if (interval > 1) {
		return (
			Math.floor(interval) + " " + pluralize(interval, "minute", "minutes")
		);
	}
	return Math.floor(seconds) + " " + pluralize(seconds, "second", "seconds");
};
