<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CalendarGrid, {
		months,
		isToday,
		isCurrentMonth,
		isDateDisabled
	} from './CalendarGrid.svelte';
	import { endOfDay, isSameDay } from 'date-fns';
	import { Calendar, X } from 'lucide-svelte';
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
		id = 'date-picker',
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
	let popover = $state<HTMLDivElement>();

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

	function handleDateClick(date: Date) {
		value = endOfDay(date);
		onChange(value);
		popover?.hidePopover();
	}

	function handleClear(e: MouseEvent) {
		e.stopPropagation();
		value = null;
		onChange(null);
	}

	function handleToggle() {
		if (disabled) return;
		popover?.togglePopover();
		if (popover?.matches(':popover-open') && value) {
			currentDate = new Date(value.getFullYear(), value.getMonth(), 1);
		}
	}

	function getDayClass(date: Date): string {
		const baseClasses =
			'w-8 h-8 flex items-center justify-center text-sm rounded-md transition-colors';

		if (isDateDisabled(date, minDate, maxDate)) {
			return twMerge(baseClasses, 'text-on-surface1 cursor-default opacity-50');
		}

		if (isSelected(date)) {
			return twMerge(baseClasses, 'bg-primary text-white font-medium');
		}

		if (isToday(date)) {
			return twMerge(baseClasses, 'border border-primary text-primary bg-primary/10');
		}

		if (!isCurrentMonth(date, currentDate)) {
			return twMerge(baseClasses, 'text-on-surface1');
		}

		return twMerge(baseClasses, 'hover:bg-surface3 cursor-pointer');
	}
</script>

<div class="date-picker-container">
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
		style={`anchor-name: --${id}-anchor`}
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

	<div
		bind:this={popover}
		class="dropdown-menu z-50 mt-1 p-4"
		popover
		id={`${id}-popover`}
		style={`position-anchor: --${id}-anchor; width: anchor-size(width); position-area: bottom; position-try-fallbacks: flip-block;`}
	>
		<div class="flex flex-col">
			<CalendarGrid
				bind:currentDate
				{minDate}
				{maxDate}
				{getDayClass}
				onDateClick={handleDateClick}
			>
				{#if clearable}
					<div class="mt-4 flex justify-end">
						<button
							type="button"
							class="button text-xs"
							onclick={() => {
								value = null;
								onChange(null);
								popover?.hidePopover();
							}}
						>
							Clear
						</button>
					</div>
				{/if}
			</CalendarGrid>
		</div>
	</div>
</div>
