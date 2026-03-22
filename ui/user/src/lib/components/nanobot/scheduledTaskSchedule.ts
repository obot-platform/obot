export type TaskFrequency = 'daily' | 'weekly' | 'monthly' | 'no_repeat';

export interface TaskScheduleForm {
	frequency: TaskFrequency;
	time: string;
	date: string;
	daysOfWeek: string[];
	daysOfMonth: number[];
}

export const weekdayOrder = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];

export const weekdayLabels: Record<string, string> = {
	sun: 'Sun',
	mon: 'Mon',
	tue: 'Tue',
	wed: 'Wed',
	thu: 'Thu',
	fri: 'Fri',
	sat: 'Sat'
};

export function ordinal(day: number) {
	if (day % 10 === 1 && day % 100 !== 11) return `${day}st`;
	if (day % 10 === 2 && day % 100 !== 12) return `${day}nd`;
	if (day % 10 === 3 && day % 100 !== 13) return `${day}rd`;
	return `${day}th`;
}

export function joinNatural(items: string[]) {
	if (items.length === 0) return '';
	if (items.length === 1) return items[0];
	if (items.length === 2) return `${items[0]} and ${items[1]}`;
	return `${items.slice(0, -1).join(', ')}, and ${items.at(-1)}`;
}

export function formatScheduleDate(date: string): string {
	if (!date) return '';

	const parsed = new Date(`${date}T00:00:00`);
	if (Number.isNaN(parsed.getTime())) {
		return date;
	}

	const month = String(parsed.getMonth() + 1).padStart(2, '0');
	const day = String(parsed.getDate()).padStart(2, '0');
	const year = parsed.getFullYear();
	return `${month}-${day}-${year}`;
}

export function formatScheduleDateTime(value?: string | null): string {
	if (!value) return 'Not available';

	const parsed = new Date(value);
	if (Number.isNaN(parsed.getTime())) {
		return value;
	}

	const date = `${String(parsed.getMonth() + 1).padStart(2, '0')}-${String(parsed.getDate()).padStart(2, '0')}-${parsed.getFullYear()}`;
	const time = `${String(parsed.getHours()).padStart(2, '0')}:${String(parsed.getMinutes()).padStart(2, '0')}`;
	return `${date} at ${time}`;
}

export function formatWeekdaySummary(daysOfWeek: string[]): string {
	return joinNatural(
		[...daysOfWeek]
			.sort((a, b) => weekdayOrder.indexOf(a) - weekdayOrder.indexOf(b))
			.map((day) => weekdayLabels[day] ?? day)
	);
}

export function formatMonthDaySummary(daysOfMonth: number[]): string {
	return joinNatural([...daysOfMonth].sort((a, b) => a - b).map((day) => ordinal(day)));
}

export function defaultTaskScheduleForm(): TaskScheduleForm {
	return {
		frequency: 'daily',
		time: '09:00',
		date: '',
		daysOfWeek: [],
		daysOfMonth: []
	};
}

export function parseCronSchedule(schedule: string, expiration?: string): TaskScheduleForm {
	const fallback = defaultTaskScheduleForm();
	const fields = schedule.trim().split(/\s+/);
	if (fields.length !== 5) return fallback;

	const [minute, hour, dayOfMonth, month, dayOfWeek] = fields;
	const time = `${hour.padStart(2, '0')}:${minute.padStart(2, '0')}`;

	if (dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
		return { ...fallback, frequency: 'daily', time };
	}

	if (dayOfMonth === '*' && month === '*' && dayOfWeek !== '*') {
		return {
			...fallback,
			frequency: 'weekly',
			time,
			daysOfWeek: dayOfWeek
				.split(',')
				.map((value) => weekdayOrder[Number(value)] ?? value.toLowerCase())
				.filter((value) => weekdayOrder.includes(value))
		};
	}

	if (dayOfMonth !== '*' && month === '*' && dayOfWeek === '*') {
		return {
			...fallback,
			frequency: 'monthly',
			time,
			daysOfMonth: dayOfMonth
				.split(',')
				.map((value) => Number(value))
				.filter((value) => Number.isInteger(value) && value >= 1 && value <= 31)
		};
	}

	if (dayOfMonth !== '*' && month !== '*' && dayOfWeek === '*' && expiration) {
		return {
			...fallback,
			frequency: 'no_repeat',
			time,
			date: expiration
		};
	}

	return fallback;
}

export function buildCronSchedule(form: TaskScheduleForm): string {
	const [hour = '09', minute = '00'] = form.time.split(':');

	switch (form.frequency) {
		case 'daily':
			return `${Number(minute)} ${Number(hour)} * * *`;
		case 'weekly':
			return `${Number(minute)} ${Number(hour)} * * ${[...form.daysOfWeek]
				.sort((a, b) => weekdayOrder.indexOf(a) - weekdayOrder.indexOf(b))
				.map((day) => weekdayOrder.indexOf(day))
				.join(',')}`;
		case 'monthly':
			return `${Number(minute)} ${Number(hour)} ${[...form.daysOfMonth].sort((a, b) => a - b).join(',')} * *`;
		case 'no_repeat': {
			const [, month, day] = form.date.split('-');
			return `${Number(minute)} ${Number(hour)} ${Number(day)} ${Number(month)} *`;
		}
	}
}

function formatTime(time: string): string {
	const [hour, minute] = time.split(':');
	return `${Number(hour)}:${minute}`;
}

export function scheduleSummary(schedule: string, expiration?: string): string {
	if (!schedule?.trim()) return 'No schedule';

	const fallback = defaultTaskScheduleForm();
	const parsed = parseCronSchedule(schedule, expiration);

	// If parsing fell back to the default but the raw cron doesn't match,
	// the schedule is unparseable — don't show a plausible-but-wrong default.
	const defaultCron = buildCronSchedule(fallback);
	if (JSON.stringify(parsed) === JSON.stringify(fallback) && schedule.trim() !== defaultCron) {
		return 'Schedule unavailable';
	}

	const time = formatTime(parsed.time);
	switch (parsed.frequency) {
		case 'daily':
			return `Daily at ${time}`;
		case 'weekly':
			return `Weekly on ${formatWeekdaySummary(parsed.daysOfWeek)} at ${time}`;
		case 'monthly':
			return `Monthly on ${formatMonthDaySummary(parsed.daysOfMonth)} at ${time}`;
		case 'no_repeat':
			return parsed.date ? `${formatScheduleDate(parsed.date)} at ${time}` : time;
	}
}
