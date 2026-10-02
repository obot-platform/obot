import { getLocale, m } from '$lib/i18n';
import type { TimeDisplayFormat } from '$lib/time';

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
	sun: m.chat_weekday_short_sun(),
	mon: m.chat_weekday_short_mon(),
	tue: m.chat_weekday_short_tue(),
	wed: m.chat_weekday_short_wed(),
	thu: m.chat_weekday_short_thu(),
	fri: m.chat_weekday_short_fri(),
	sat: m.chat_weekday_short_sat()
};

export function ordinal(day: number) {
	if (day % 10 === 1 && day % 100 !== 11) return m.chat_ordinal_st({ day });
	if (day % 10 === 2 && day % 100 !== 12) return m.chat_ordinal_nd({ day });
	if (day % 10 === 3 && day % 100 !== 13) return m.chat_ordinal_rd({ day });
	return m.chat_ordinal_th({ day });
}

export function joinNatural(items: string[]) {
	if (items.length === 0) return '';
	if (items.length === 1) return items[0];
	if (items.length === 2) return m.chat_join_two({ first: items[0], second: items[1] });
	return m.chat_join_many({
		items: items.slice(0, -1).join(m.chat_list_separator()),
		last: items.at(-1) ?? ''
	});
}

export function formatScheduleDate(date: string): string {
	if (!date) return '';

	const parsed = new Date(`${date}T00:00:00`);
	if (Number.isNaN(parsed.getTime())) {
		return date;
	}

	return new Intl.DateTimeFormat(getLocale(), {
		year: 'numeric',
		month: 'numeric',
		day: 'numeric'
	}).format(parsed);
}

export function formatScheduleDateTime(
	value: string | null | undefined,
	format: TimeDisplayFormat
): string {
	if (!value) return m.chat_not_available();

	const parsed = new Date(value);
	if (Number.isNaN(parsed.getTime())) {
		return value;
	}

	return new Intl.DateTimeFormat(getLocale(), {
		year: 'numeric',
		month: 'numeric',
		day: 'numeric',
		hour: '2-digit',
		minute: '2-digit',
		hour12: format === '12h'
	}).format(parsed);
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
	// Reject cron expressions with non-numeric or out-of-range time fields (e.g. `*`, `*/5`, ranges)
	const min = Number(minute);
	const hr = Number(hour);
	if (!Number.isInteger(min) || !Number.isInteger(hr) || min < 0 || min > 59 || hr < 0 || hr > 23) {
		return fallback;
	}
	const time = `${String(hr).padStart(2, '0')}:${String(min).padStart(2, '0')}`;

	if (dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
		return { ...fallback, frequency: 'daily', time };
	}

	if (dayOfMonth === '*' && month === '*' && dayOfWeek !== '*') {
		return {
			...fallback,
			frequency: 'weekly',
			time,
			// `% 7` normalizes cron day 7 (alternate Sunday) to 0
			daysOfWeek: dayOfWeek
				.split(',')
				.map((value) => weekdayOrder[Number(value) % 7] ?? value.toLowerCase())
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
			// Expects YYYY-MM-DD; falls back to daily-at-9 if date is missing/malformed
			const parts = form.date.split('-');
			const month = Number(parts[1]);
			const day = Number(parts[2]);
			if (!Number.isInteger(month) || !Number.isInteger(day)) return '0 9 * * *';
			return `${Number(minute)} ${Number(hour)} ${day} ${month} *`;
		}
	}
}

function formatScheduleTime(time: string, format: TimeDisplayFormat): string {
	const parts = time.split(':');
	const hour = Number(parts[0]);
	const minute = Number(parts[1] ?? 0);
	if (
		!Number.isInteger(hour) ||
		!Number.isInteger(minute) ||
		hour < 0 ||
		hour > 23 ||
		minute < 0 ||
		minute > 59
	) {
		return time;
	}

	const d = new Date();
	d.setHours(hour, minute, 0, 0);
	return new Intl.DateTimeFormat(getLocale(), {
		hour: '2-digit',
		minute: '2-digit',
		hour12: format === '12h'
	}).format(d);
}

export function scheduleSummary(
	schedule: string,
	expiration: string | undefined,
	format: TimeDisplayFormat
): string {
	if (!schedule?.trim()) return m.chat_no_schedule();

	const fallback = defaultTaskScheduleForm();
	const parsed = parseCronSchedule(schedule, expiration);

	// If parsing fell back to the default but the raw cron doesn't match,
	// the schedule is unparseable — don't show a plausible-but-wrong default.
	const defaultCron = buildCronSchedule(fallback);
	if (JSON.stringify(parsed) === JSON.stringify(fallback) && schedule.trim() !== defaultCron) {
		return m.chat_schedule_unavailable();
	}

	const time = formatScheduleTime(parsed.time, format);
	switch (parsed.frequency) {
		case 'daily':
			return m.chat_schedule_daily_at({ time });
		case 'weekly':
			// Guard against empty weekday list producing "Weekly on at 9:00"
			if (!parsed.daysOfWeek.length) return m.chat_schedule_unavailable();
			return m.chat_schedule_weekly_on({ days: formatWeekdaySummary(parsed.daysOfWeek), time });
		case 'monthly':
			if (!parsed.daysOfMonth.length) return m.chat_schedule_unavailable();
			return m.chat_schedule_monthly_on({ days: formatMonthDaySummary(parsed.daysOfMonth), time });
		case 'no_repeat':
			return parsed.date
				? m.chat_schedule_date_at({ date: formatScheduleDate(parsed.date), time })
				: time;
	}
}

export function estimateNextRun(schedule: string, expiration?: string): Date {
	const now = new Date();
	const parsed = parseCronSchedule(schedule, expiration);
	if (!parsed) return now;
	now.setHours(Number(parsed.time.split(':')[0]), Number(parsed.time.split(':')[1]), 0, 0);
	return now;
}
