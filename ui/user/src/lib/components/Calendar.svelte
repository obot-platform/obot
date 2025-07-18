<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { ChevronLeft, ChevronRight, CalendarCog } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	export interface DateRange {
		start: Date | null;
		end: Date | null;
	}

	interface Props {
		id?: string;
		disabled?: boolean;
		initialValue?: DateRange;
		onChange: (range: DateRange) => void;
		class?: string;
		classes?: {
			root?: string;
			calendar?: string;
			header?: string;
			grid?: string;
			day?: string;
		};
		minDate?: Date;
		maxDate?: Date;
		placeholder?: string;
		format?: string;
		compact?: boolean;
	}

	const {
		id,
		disabled,
		initialValue = { start: null, end: null },
		onChange,
		class: klass,
		classes,
		minDate,
		maxDate,
		placeholder = 'Select date range',
		format = 'MMM dd, yyyy',
		compact
	}: Props = $props();

	let currentDate = $state(new Date());
	let popover = $state<HTMLDialogElement>();

	// Local state for the date range being edited
	let localValue = $state<DateRange>({ ...initialValue });

	// Get current month's first day and last day
	let firstDayOfMonth = $derived(new Date(currentDate.getFullYear(), currentDate.getMonth(), 1));
	let startOfWeek = $derived.by(() => {
		const date = new Date(firstDayOfMonth);
		date.setDate(date.getDate() - date.getDay());
		return date;
	});

	function generateCalendarDays(): Date[] {
		const days: Date[] = [];
		const current = new Date(startOfWeek);

		// Generate 6 weeks of days (42 days)
		for (let i = 0; i < 42; i++) {
			days.push(new Date(current));
			current.setDate(current.getDate() + 1);
		}

		return days;
	}

	let calendarDays = $derived(generateCalendarDays());

	const weekdays = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
	const months = [
		'January',
		'February',
		'March',
		'April',
		'May',
		'June',
		'July',
		'August',
		'September',
		'October',
		'November',
		'December'
	];

	function formatDate(date: Date): string {
		if (!date) return '';

		const day = date.getDate().toString().padStart(2, '0');
		const month = (date.getMonth() + 1).toString().padStart(2, '0');
		const year = date.getFullYear();

		return format
			.replace('dd', day)
			.replace('MM', month)
			.replace('MMM', months[date.getMonth()].substring(0, 3))
			.replace('yyyy', year.toString());
	}

	function formatRange(): string {
		if (!localValue.start && !localValue.end) return placeholder;
		if (localValue.start && !localValue.end)
			return `${formatDate(localValue.start)} - Select end date`;
		if (!localValue.start && localValue.end)
			return `Select start date - ${formatDate(localValue.end)}`;
		if (localValue.start && localValue.end)
			return `${formatDate(localValue.start)} - ${formatDate(localValue.end)}`;
		return placeholder;
	}

	function isInRange(date: Date): boolean {
		if (!localValue.start || !localValue.end) return false;
		return date >= localValue.start && date <= localValue.end;
	}

	function isStartDate(date: Date): boolean {
		return localValue.start ? date.toDateString() === localValue.start.toDateString() : false;
	}

	function isEndDate(date: Date): boolean {
		return localValue.end ? date.toDateString() === localValue.end.toDateString() : false;
	}

	function isToday(date: Date): boolean {
		const today = new Date();
		return date.toDateString() === today.toDateString();
	}

	function isCurrentMonth(date: Date): boolean {
		return (
			date.getMonth() === currentDate.getMonth() && date.getFullYear() === currentDate.getFullYear()
		);
	}

	function isDisabled(date: Date): boolean {
		if (minDate && date < minDate) return true;
		if (maxDate && date > maxDate) return true;
		return false;
	}

	function handleDateClick(date: Date) {
		if (isDisabled(date)) return;

		let newRange: DateRange;

		if (!localValue.start || (localValue.start && localValue.end)) {
			// Start new range
			newRange = { start: date, end: null };
		} else {
			// Complete the range
			if (date < localValue.start) {
				newRange = { start: date, end: localValue.start };
			} else {
				newRange = { start: localValue.start, end: date };
			}
		}

		localValue = newRange;
	}

	function previousMonth() {
		currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() - 1, 1);
	}

	function nextMonth() {
		currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() + 1, 1);
	}

	function handleApply() {
		onChange(localValue);
		popover?.close();
	}

	function handleCancel() {
		// Reset local value to initial value
		localValue = { ...initialValue };
		popover?.close();
	}

	function getDayClass(date: Date): string {
		const baseClasses =
			'w-8 h-8 flex items-center justify-center text-sm rounded-md transition-colors';

		if (isDisabled(date)) {
			return twMerge(baseClasses, 'text-gray-400 cursor-not-allowed');
		}

		if (isStartDate(date) || isEndDate(date)) {
			return twMerge(baseClasses, 'bg-blue-500 text-white font-medium');
		}

		if (isInRange(date)) {
			return twMerge(baseClasses, 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300');
		}

		if (isToday(date)) {
			return twMerge(baseClasses, 'border border-blue-500 text-blue-600 dark:text-blue-400');
		}

		if (!isCurrentMonth(date)) {
			return twMerge(baseClasses, 'text-gray-400');
		}

		return twMerge(baseClasses, 'hover:bg-gray-100 dark:hover:bg-gray-700 cursor-pointer');
	}
</script>

<div class={twMerge('relative', classes?.root)}>
	<button
		{id}
		{disabled}
		class={twMerge(
			'dark:bg-surface1 text-md flex min-h-10 w-full grow resize-none items-center justify-between rounded-lg bg-white px-4 py-2 text-left shadow-sm',
			disabled && 'cursor-not-allowed opacity-50',
			klass
		)}
		onmousedown={() => {
			if (disabled) return;
			if (popover?.open) {
				popover?.close();
			} else {
				popover?.show();
			}
		}}
		use:tooltip={{
			text: 'Filter By Date',
			placement: 'top-end'
		}}
	>
		<span class="text-md flex grow items-center gap-2 truncate">
			<CalendarCog class="size-4" />
			{#if !compact}
				{formatRange()}
			{/if}
		</span>
	</button>

	<dialog
		use:clickOutside={[() => popover?.close(), true]}
		bind:this={popover}
		class={twMerge(
			'default-dialog absolute top-full left-12 z-50 mt-1 min-w-[320px] -translate-x-full p-4',
			classes?.calendar
		)}
	>
		<!-- Calendar Header -->
		<div class={twMerge('mb-4 flex items-center justify-between', classes?.header)}>
			<button class="rounded p-1 hover:bg-gray-100 dark:hover:bg-gray-700" onclick={previousMonth}>
				<ChevronLeft class="size-4" />
			</button>

			<h3 class="text-lg font-semibold">
				{months[currentDate.getMonth()]}
				{currentDate.getFullYear()}
			</h3>

			<button class="rounded p-1 hover:bg-gray-100 dark:hover:bg-gray-700" onclick={nextMonth}>
				<ChevronRight class="size-4" />
			</button>
		</div>

		<!-- Weekday Headers -->
		<div class="mb-2 grid grid-cols-7 gap-1">
			{#each weekdays as day, i (i)}
				<div class="flex h-8 w-8 items-center justify-center text-xs font-medium text-gray-500">
					{day}
				</div>
			{/each}
		</div>

		<!-- Calendar Grid -->
		<div class={twMerge('grid grid-cols-7 gap-1', classes?.grid)}>
			{#each calendarDays as date (date.toISOString())}
				<button
					class={getDayClass(date)}
					onclick={() => handleDateClick(date)}
					disabled={isDisabled(date)}
				>
					{date.getDate()}
				</button>
			{/each}
		</div>

		<div class="mt-4 flex justify-end gap-2">
			<button class="button text-xs" onclick={handleCancel}>Cancel</button>
			<button class="button-primary text-xs" onclick={handleApply}>Apply</button>
		</div>
	</dialog>
</div>
