<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { endOfDay, isSameDay, startOfDay } from 'date-fns';
	import { ChevronLeft, ChevronRight, Calendar, X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id?: string;
		disabled?: boolean;
		value?: Date | null;
		onChange: (date: Date | null) => void;
		class?: string;
		minDate?: Date;
		maxDate?: Date;
		placeholder?: string;
		format?: string;
		clearable?: boolean;
	}

	let {
		id,
		disabled,
		value = $bindable(null),
		onChange,
		class: klass,
		minDate,
		maxDate,
		placeholder = 'Select date',
		format = 'MMM dd, yyyy',
		clearable = true
	}: Props = $props();

	let currentDate = $state(new Date());
	let open = $state(false);

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

		// Replace MMM before MM (more specific pattern first)
		return format
			.replace('MMM', months[date.getMonth()].substring(0, 3))
			.replace('MM', month)
			.replace('dd', day)
			.replace('yyyy', year.toString());
	}

	function isSelected(date: Date): boolean {
		return value ? isSameDay(date, value) : false;
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
		if (minDate && date < startOfDay(minDate)) return true;
		if (maxDate && date > endOfDay(maxDate)) return true;
		return false;
	}

	function handleDateClick(date: Date) {
		if (isDisabled(date)) return;
		value = endOfDay(date);
		onChange(value);
		open = false;
	}

	function handleClear(e: MouseEvent) {
		e.stopPropagation();
		value = null;
		onChange(null);
	}

	function handleToggle() {
		if (disabled) return;
		open = !open;
		if (open && value) {
			currentDate = new Date(value.getFullYear(), value.getMonth(), 1);
		}
	}

	function previousMonth() {
		currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() - 1, 1);
	}

	function nextMonth() {
		currentDate = new Date(currentDate.getFullYear(), currentDate.getMonth() + 1, 1);
	}

	function getDayClass(date: Date): string {
		const baseClasses =
			'w-8 h-8 flex items-center justify-center text-sm rounded-md transition-colors';

		if (isDisabled(date)) {
			return twMerge(baseClasses, 'text-on-surface1 cursor-default opacity-50');
		}

		if (isSelected(date)) {
			return twMerge(baseClasses, 'bg-primary text-white font-medium');
		}

		if (isToday(date)) {
			return twMerge(baseClasses, 'border border-primary text-primary bg-primary/10');
		}

		if (!isCurrentMonth(date)) {
			return twMerge(baseClasses, 'text-on-surface1');
		}

		return twMerge(baseClasses, 'hover:bg-surface3 cursor-pointer');
	}

	function handleClickOutside(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (!target.closest('.date-picker-container')) {
			open = false;
		}
	}

	$effect(() => {
		if (open) {
			document.addEventListener('click', handleClickOutside, true);
			return () => document.removeEventListener('click', handleClickOutside, true);
		}
	});
</script>

<div class="date-picker-container relative">
	<button
		{id}
		{disabled}
		type="button"
		class={twMerge(
			'text-input-filled flex min-h-10 w-full items-center justify-between gap-2',
			disabled && 'cursor-default opacity-50',
			klass
		)}
		onclick={handleToggle}
	>
		<span class="flex grow items-center gap-2 truncate">
			<Calendar class="text-on-surface1 size-4 flex-shrink-0" />
			<span class={twMerge(!value && 'text-on-surface1')}>
				{value ? formatDate(value) : placeholder}
			</span>
		</span>
		{#if clearable && value && !disabled}
			<span
				role="button"
				tabindex="0"
				class="hover:bg-surface3 -mr-1 rounded p-1"
				onclick={handleClear}
				onkeydown={(e) => e.key === 'Enter' && handleClear(e as unknown as MouseEvent)}
				{@attach (node: HTMLElement) => {
					const response = tooltip(node, {
						text: 'Clear',
						placement: 'top'
					});
					return () => response.destroy();
				}}
			>
				<X class="size-4" />
			</span>
		{/if}
	</button>

	{#if open}
		<div class="default-dialog absolute top-full z-50 mt-1 flex flex-col p-4">
			<!-- Calendar Header -->
			<div class="mb-4 flex items-center justify-between">
				<button type="button" class="hover:bg-surface3 rounded p-1" onclick={previousMonth}>
					<ChevronLeft class="size-4" />
				</button>

				<h3 class="text-sm font-medium">
					{months[currentDate.getMonth()]}
					{currentDate.getFullYear()}
				</h3>

				<button type="button" class="hover:bg-surface3 rounded p-1" onclick={nextMonth}>
					<ChevronRight class="size-4" />
				</button>
			</div>

			<!-- Weekday Headers -->
			<div class="mb-2 grid grid-cols-7 gap-1">
				{#each weekdays as day, i (i)}
					<div
						class="text-on-surface1 flex h-8 w-8 items-center justify-center text-xs font-medium"
					>
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

			{#if clearable}
				<div class="mt-4 flex justify-end">
					<button
						type="button"
						class="button text-xs"
						onclick={() => {
							value = null;
							onChange(null);
							open = false;
						}}
					>
						Clear
					</button>
				</div>
			{/if}
		</div>
	{/if}
</div>
