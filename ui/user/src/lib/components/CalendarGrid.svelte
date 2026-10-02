<script lang="ts" module>
	import { getLocale } from '$lib/i18n';
	import { startOfDay, endOfDay } from 'date-fns';

	// Month and weekday names come from Intl so they follow the active locale.
	const nameFormat = (options: Intl.DateTimeFormatOptions) => {
		const format = new Intl.DateTimeFormat(getLocale(), { ...options, timeZone: 'UTC' });
		return (date: Date) => format.format(date);
	};
	const monthName = nameFormat({ month: 'short' });
	const weekdayName = nameFormat({ weekday: 'short' });

	export const monthsShort = Array.from({ length: 12 }, (_, i) =>
		monthName(new Date(Date.UTC(2024, i, 1)))
	);

	// 2024-09-01 was a Sunday.
	export const weekdays = Array.from({ length: 7 }, (_, i) =>
		weekdayName(new Date(Date.UTC(2024, 8, 1 + i)))
	);

	export function formatMonthYear(date: Date): string {
		return new Intl.DateTimeFormat(getLocale(), { month: 'long', year: 'numeric' }).format(date);
	}

	export function isToday(date: Date): boolean {
		const today = new Date();
		return date.toDateString() === today.toDateString();
	}

	export function isCurrentMonth(date: Date, currentDate: Date): boolean {
		return (
			date.getMonth() === currentDate.getMonth() && date.getFullYear() === currentDate.getFullYear()
		);
	}

	export function isDateDisabled(date: Date, minDate?: Date, maxDate?: Date): boolean {
		if (minDate && date < startOfDay(minDate)) return true;
		if (maxDate && date > endOfDay(maxDate)) return true;
		return false;
	}
</script>

<script lang="ts">
	import { ChevronLeft, ChevronRight } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		currentDate?: Date;
		minDate?: Date;
		maxDate?: Date;
		getDayClass: (date: Date) => string;
		onDateClick: (date: Date) => void;
		class?: string;
		children?: Snippet;
	}

	let {
		currentDate = $bindable(new Date()),
		minDate,
		maxDate,
		getDayClass,
		onDateClick,
		class: klass,
		children
	}: Props = $props();

	// Get current month's first day
	let firstDayOfMonth = $derived(new Date(currentDate.getFullYear(), currentDate.getMonth(), 1));
	let startOfWeek = $derived(
		new Date(
			firstDayOfMonth.getFullYear(),
			firstDayOfMonth.getMonth(),
			firstDayOfMonth.getDate() - firstDayOfMonth.getDay()
		)
	);

	function generateCalendarDays(): Date[] {
		const days: Date[] = [];
		for (let i = 0; i < 42; i++) {
			days.push(
				new Date(startOfWeek.getFullYear(), startOfWeek.getMonth(), startOfWeek.getDate() + i)
			);
		}
		return days;
	}

	let calendarDays = $derived(generateCalendarDays());

	function previousMonth() {
		currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() - 1, 1);
	}

	function nextMonth() {
		currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() + 1, 1);
	}

	function isDisabled(date: Date): boolean {
		return isDateDisabled(date, minDate, maxDate);
	}

	function handleDateClick(date: Date) {
		if (isDisabled(date)) return;
		onDateClick(date);
	}
</script>

<div class={twMerge('flex flex-col', klass)}>
	<!-- Calendar Header -->
	<div class="mb-4 flex items-center justify-between">
		<button type="button" class="hover:bg-base-400 rounded p-1" onclick={previousMonth}>
			<ChevronLeft class="size-4" />
		</button>

		<h3 class="text-sm font-medium">
			{formatMonthYear(currentDate)}
		</h3>

		<button type="button" class="hover:bg-base-400 rounded p-1" onclick={nextMonth}>
			<ChevronRight class="size-4" />
		</button>
	</div>

	<!-- Weekday Headers -->
	<div class="mb-2 grid grid-cols-7 gap-1">
		{#each weekdays as day, i (i)}
			<div class="text-muted-content flex h-8 w-8 items-center justify-center text-xs font-medium">
				{day}
			</div>
		{/each}
	</div>

	<!-- Calendar Grid -->
	<div class="grid grid-cols-7 gap-1">
		{#each calendarDays as date (date.toISOString())}
			<button
				type="button"
				class={getDayClass(date)}
				onclick={() => handleDateClick(date)}
				disabled={isDisabled(date)}
			>
				{date.getDate()}
			</button>
		{/each}
	</div>

	<!-- Footer slot -->
	{@render children?.()}
</div>
