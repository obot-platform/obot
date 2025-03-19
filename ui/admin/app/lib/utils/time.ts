import { pluralize } from "~/lib/utils/pluralize";

export const timeSince = (date: Date) => {
	const seconds = Math.floor((new Date().getTime() - date.getTime()) / 1000);

	let interval = seconds / 31536000;

	if (interval > 1) {
		interval = Math.floor(interval);
		return interval + " " + pluralize(interval, "year", "years");
	}
	interval = seconds / 2592000;
	if (interval > 1) {
		interval = Math.floor(interval);
		return interval + " " + pluralize(interval, "month", "months");
	}
	interval = seconds / 86400;
	if (interval > 1) {
		interval = Math.floor(interval);
		return interval + " " + pluralize(interval, "day", "days");
	}
	interval = seconds / 3600;
	if (interval > 1) {
		interval = Math.floor(interval);
		return interval + " " + pluralize(interval, "hour", "hours");
	}
	interval = seconds / 60;
	if (interval > 1) {
		interval = Math.floor(interval);
		return interval + " " + pluralize(interval, "minute", "minutes");
	}
	interval = Math.floor(seconds);
	return interval + " " + pluralize(interval, "second", "seconds");
};

export const daysSince = (date: Date) => {
	const seconds = Math.floor(
		(new Date().getTime() - Math.floor(date.getTime() / 86400000) * 86400000) /
			1000
	);

	let interval = seconds / 31536000;

	if (interval > 1) {
		interval = Math.floor(interval);
		return interval + " " + pluralize(interval, "year", "years") + " ago";
	}
	interval = seconds / 2592000;
	if (interval > 1) {
		interval = Math.floor(interval);
		return interval + " " + pluralize(interval, "month", "months") + " ago";
	}
	interval = seconds / 86400;
	if (interval > 1) {
		interval = Math.floor(interval);
		return interval + " " + pluralize(interval, "day", "days") + " ago";
	}
	return "Today";
};

export const formatTime = (time: Date | string) => {
	const now = new Date();
	if (typeof time === "string") {
		time = new Date(time);
	}
	if (
		time.getDate() == now.getDate() &&
		time.getMonth() == now.getMonth() &&
		time.getFullYear() == now.getFullYear()
	) {
		return time.toLocaleTimeString(undefined, {
			hour: "numeric",
			minute: "numeric",
		});
	}
	return time.toLocaleDateString(undefined, {
		month: "short",
		day: "numeric",
		hour: "numeric",
		minute: "numeric",
	});
};
