<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import popover from '$lib/actions/popover.svelte';
	import CalendarGrid, {
		months,
		isToday,
		isCurrentMonth,
		isDateDisabled
	} from './CalendarGrid.svelte';
	import { differenceInDays, endOfDay, isBefore, isSameDay, startOfDay } from 'date-fns';
	import { CalendarCog } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import TimeInput from './TimeInput.svelte';
	import { slide } from 'svelte/transition';
	import { responsive } from '$lib/stores';

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
		start: Date | null;
		end: Date | null;
		minDate?: Date;
		maxDate?: Date;
		placeholder?: string;
		format?: string;
		compact?: boolean;
		open?: boolean;
	}

	let {
		id,
		disabled,
		initialValue = { start: null, end: null },
		onChange,
		class: klass,
		classes,
		minDate,
		maxDate,
		start = $bindable(initialValue.start),
		end = $bindable(initialValue.end),
		placeholder = 'Select date range',
		format = 'MMM dd, yyyy',
		compact,
		open = $bindable(false)
	}: Props = $props();

	let currentDate = $state(new Date());

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
		if (!start && !end) return placeholder;
		if (start && !end) return `${formatDate(start)} - Select end date`;
		if (!start && end) return `Select start date - ${formatDate(end)}`;
		if (start && end) return `${formatDate(start)} - ${formatDate(end)}`;
		return placeholder;
	}

	function isInRange(date: Date): boolean {
		if (!start || !end) return false;
		return date >= start && date <= end;
	}

	function isStartDate(date: Date): boolean {
		return start ? date.toDateString() === start.toDateString() : false;
	}

	function isEndDate(date: Date): boolean {
		return end ? date.toDateString() === end.toDateString() : false;
	}

	function handleDateClick(date: Date) {
		if (!start || (start && isSameDay(date, start)) || (end && isSameDay(date, end))) {
			// If clicked date is both start or end, collapse the range to that date
			start = startOfDay(date);
			end = endOfDay(date);
		} else if (start) {
			if (isBefore(date, start)) {
				// If clicked date is before start date, expand the range backwards
				start = startOfDay(date);
			} else {
				// If clicked date is after start date, expand the range forwards
				end = endOfDay(date);
			}
		}
	}

	function handleApply() {
		onChange({ start, end });

		open = false;
	}

	function handleCancel() {
		// Reset local value to initial value
		start = initialValue.start;
		end = initialValue.end;

		open = false;
	}

	function getDayClass(date: Date): string {
		const baseClasses =
			'w-8 h-8 flex items-center justify-center text-sm rounded-md transition-colors';

		if (isDateDisabled(date, minDate, maxDate)) {
			return twMerge(baseClasses, 'text-on-surface1 cursor-default');
		}

		if (isStartDate(date) || isEndDate(date)) {
			return twMerge(baseClasses, 'bg-primary text-white font-medium');
		}

		if (isInRange(date)) {
			return twMerge(baseClasses, 'bg-primary/10 text-primary');
		}

		if (isToday(date)) {
			return twMerge(baseClasses, 'border border-primary text-primary bg-primary/10');
		}

		if (!isCurrentMonth(date, currentDate)) {
			return twMerge(baseClasses, 'text-on-surface1');
		}

		return twMerge(baseClasses, 'hover:bg-surface3 cursor-pointer');
	}

	const calendarPopover = popover({
		placement: 'bottom-end',
		offset: 4,
		onOpenChange: (isOpen) => {
			open = isOpen;
		}
	});

	const isSmallScreen = $derived(responsive.isMobile);

	function popoverRef(node: HTMLElement) {
		return calendarPopover.ref(node);
	}

	function tooltipRef(node: HTMLElement) {
		if (isSmallScreen) {
			return;
		}

		return calendarPopover.tooltip(node);
	}

	// Sync open state with popover
	$effect(() => {
		calendarPopover.toggle(open);
	});
</script>

<button
	{id}
	{disabled}
	type="button"
	class={twMerge(
		'dark:bg-surface1 text-md bg-background flex min-h-10 resize-none items-center justify-between rounded-lg px-4 py-2 text-left shadow-sm',
		disabled && 'cursor-default opacity-50',
		klass
	)}
	use:popoverRef
	onclick={() => !disabled && calendarPopover.toggle()}
	{@attach (node: HTMLElement) => {
		const response = tooltip(node, {
			text: 'Filter By Date',
			placement: 'top-end'
		});

		return () => response.destroy();
	}}
>
	<span class="text-md flex grow items-center gap-2 truncate">
		<CalendarCog class="size-4" />
		{#if !compact}
			{formatRange()}
		{/if}
	</span>
</button>

<div
	class={twMerge(
		'flex flex-col items-center justify-center',
		isSmallScreen ? 'fixed inset-0 z-50 min-w-full p-4 backdrop-blur-xs' : 'contents',
		!open && 'hidden'
	)}
	role="button"
	tabindex="-1"
	onclick={(ev) => {
		if (
			ev.target &&
			ev.currentTarget.contains(ev.target as HTMLElement) &&
			!(ev.currentTarget === ev.target)
		) {
			return;
		}
		calendarPopover.toggle(false);
	}}
	onkeydown={undefined}
>
	{#key isSmallScreen}
		<div
			class={twMerge(
				'default-dialog flex flex-col p-4',
				isSmallScreen && 'w-full max-w-sm',
				!isSmallScreen && 'max-w-xs',
				classes?.calendar
			)}
			use:tooltipRef
		>
			<div class="mb-6 px-4 text-center text-lg font-medium md:hidden md:text-start">
				<div>Select Export Time Range</div>
			</div>

			<CalendarGrid
				bind:currentDate
				{minDate}
				{maxDate}
				{getDayClass}
				onDateClick={handleDateClick}
			>
				{#if (start && !end) || (start && end && differenceInDays(end, start) <= 20)}
					<!-- Render Time pickers -->
					<div
						class="mt-4 flex flex-col gap-2"
						in:slide={{ duration: 200 }}
						out:slide={{ duration: 100 }}
					>
						<div class="flex flex-col gap-1">
							<div class="text-on-surface1 text-xs">{start.toDateString()}</div>
							<TimeInput
								date={start}
								onChange={(date) => {
									start = date;
								}}
							/>
						</div>

						<div class="flex flex-col gap-1">
							<!-- In case start and end dates in the same day do not render the label -->
							{#if !isSameDay(end ?? start, start)}
								<div
									class="text-on-surface1 text-xs"
									in:slide={{ duration: 200 }}
									out:slide={{ duration: 100 }}
								>
									{end?.toDateString()}
								</div>
							{/if}

							<TimeInput
								date={end ?? endOfDay(start)}
								onChange={(date) => {
									end = date;
								}}
							/>
						</div>
					</div>
				{/if}

				<div class="mt-4 flex justify-end gap-2">
					<button type="button" class="button text-xs" onclick={handleCancel}>Cancel</button>
					<button type="button" class="button-primary text-xs" onclick={handleApply}>Apply</button>
				</div>
			</CalendarGrid>
		</div>
	{/key}
</div>
