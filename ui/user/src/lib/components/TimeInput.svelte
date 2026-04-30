<script lang="ts">
	import TimeClockPopover, { type AnchorPlacement } from './TimeClockPopover.svelte';
	import {
		addHours,
		addMinutes,
		getHours,
		getMinutes,
		setHours,
		setMinutes,
		subHours,
		subMinutes
	} from 'date-fns';
	import { twMerge } from 'tailwind-merge';

	type Props = {
		date: Date;
		onChange?: (date: Date) => void;
		class?: string;
		format?: '12h' | '24h';
		clockAnchorPlacement?: AnchorPlacement;
	};

	let {
		date = $bindable(),
		onChange,
		class: klass,
		format = '12h',
		clockAnchorPlacement
	}: Props = $props();

	const hours = $derived(getHours(date));
	const minutes = $derived(getMinutes(date));
	const isAm = $derived(hours < 12);
	const hour12 = $derived(hours % 12 === 0 ? 12 : hours % 12);
	const displayHour = $derived(format === '24h' ? hours : hour12);

	let hourDraft = $state('');
	let minuteDraft = $state('');

	$effect(() => {
		hourDraft = displayHour.toString().padStart(2, '0');
	});
	$effect(() => {
		minuteDraft = (minutes % 60).toString().padStart(2, '0');
	});

	function commitHour() {
		const trimmed = hourDraft.trim();
		if (trimmed === '') {
			hourDraft = displayHour.toString().padStart(2, '0');
			return;
		}
		if (format === '24h') {
			let h24 = parseInt(trimmed, 10);
			if (Number.isNaN(h24)) {
				hourDraft = hours.toString().padStart(2, '0');
				return;
			}
			h24 = Math.min(23, Math.max(0, h24));
			date = setHours(date, h24);
			onChange?.(date);
			return;
		}

		let h12 = parseInt(trimmed, 10);
		if (Number.isNaN(h12)) {
			hourDraft = hour12.toString().padStart(2, '0');
			return;
		}
		if (trimmed === '00') {
			h12 = 12;
		} else {
			h12 = Math.min(12, Math.max(1, h12));
		}

		let h24: number;
		if (isAm) {
			h24 = h12 === 12 ? 0 : h12;
		} else {
			h24 = h12 === 12 ? 12 : h12 + 12;
		}
		date = setHours(date, h24);
		onChange?.(date);
	}

	function commitMinute() {
		const trimmed = minuteDraft.trim();
		if (trimmed === '') {
			minuteDraft = (minutes % 60).toString().padStart(2, '0');
			return;
		}
		const m = parseInt(trimmed, 10);
		if (Number.isNaN(m)) {
			minuteDraft = (minutes % 60).toString().padStart(2, '0');
			return;
		}
		date = setMinutes(date, Math.min(59, Math.max(0, m)));
		onChange?.(date);
	}

	function sanitizeHourDraft(raw: string): string {
		const v = raw.replace(/\D/g, '').slice(0, 2);
		if (v === '') return v;
		if (format === '24h') {
			const n = parseInt(v, 10);
			if (!Number.isNaN(n) && n > 23) {
				return '23';
			}
			return v;
		}
		if (v === '00') {
			return '12';
		}
		const n = parseInt(v, 10);
		if (!Number.isNaN(n) && n > 12) {
			return '12';
		}
		return v;
	}

	function sanitizeMinuteDraft(raw: string): string {
		const v = raw.replace(/\D/g, '').slice(0, 2);
		if (v === '') return v;
		const n = parseInt(v, 10);
		if (!Number.isNaN(n) && n > 59) {
			return '59';
		}
		return v;
	}

	let clockOpen = $state(false);
	let clockAnchor = $state<HTMLElement | null>(null);
	let clockInitialMode = $state<'hour' | 'minute'>('hour');

	function openClockPicker(target: HTMLInputElement, mode: 'hour' | 'minute') {
		clockAnchor = target;
		clockInitialMode = mode;
		clockOpen = true;
	}

	function onClockOpenChange(open: boolean) {
		clockOpen = open;
		if (!open) clockAnchor = null;
	}
</script>

<div class={twMerge('time-input bg-base-200 flex h-14 items-center gap-2 rounded-md', klass)}>
	<div class="flex h-full flex-1 text-xl">
		<input
			class="w-[3ch] flex-1 bg-transparent px-4 text-end"
			type="text"
			inputmode="numeric"
			autocomplete="off"
			maxlength="2"
			bind:value={hourDraft}
			onfocus={(ev) => openClockPicker(ev.currentTarget, 'hour')}
			oninput={(ev) => {
				hourDraft = sanitizeHourDraft(ev.currentTarget.value);
			}}
			onblur={commitHour}
			onkeydown={(ev) => {
				if (['ArrowDown', 'ArrowUp'].includes(ev.key)) {
					ev.preventDefault();
					return;
				}

				if (ev.key === 'Enter') {
					commitHour();
				}
			}}
			onkeyup={(ev) => {
				if (ev.key === 'ArrowDown') {
					date = subHours(date, 1);
					onChange?.(date);
				} else if (ev.key === 'ArrowUp') {
					date = addHours(date, 1);
					onChange?.(date);
				}
			}}
		/>
	</div>

	<div class="text-2xl font-bold">:</div>

	<div class="flex h-full flex-1 rounded-md text-xl">
		<input
			class="w-[3ch] flex-1 bg-transparent px-4"
			type="text"
			inputmode="numeric"
			autocomplete="off"
			maxlength="2"
			bind:value={minuteDraft}
			onfocus={(ev) => openClockPicker(ev.currentTarget, 'minute')}
			oninput={(ev) => {
				minuteDraft = sanitizeMinuteDraft(ev.currentTarget.value);
			}}
			onblur={commitMinute}
			onkeydown={(ev) => {
				if (['ArrowDown', 'ArrowUp'].includes(ev.key)) {
					ev.preventDefault();
					return;
				}

				if (ev.key === 'Enter') {
					commitMinute();
				}
			}}
			onkeyup={(ev) => {
				if (ev.key === 'ArrowDown') {
					date = subMinutes(date, 1);
					onChange?.(date);
				} else if (ev.key === 'ArrowUp') {
					date = addMinutes(date, 1);
					onChange?.(date);
				}
			}}
		/>
	</div>

	{#if format === '12h'}
		<div class="flex h-full flex-col gap-1 p-1 text-xs">
			<button
				class={twMerge(
					'bg-base-400/30 flex-1 rounded-sm px-1',
					isAm && 'bg-primary/10 border-primary/50 text-primary'
				)}
				onclick={() => {
					if (isAm) return;
					date = setHours(date, hours - 12);
					onChange?.(date);
				}}>AM</button
			>

			<button
				class={twMerge(
					'bg-base-400/30 flex-1 rounded-sm px-1',
					!isAm && 'text-primary bg-primary/10'
				)}
				onclick={() => {
					if (!isAm) return;
					date = setHours(date, (hours + 12) % 24);
					onChange?.(date);
				}}>PM</button
			>
		</div>
	{/if}

	<TimeClockPopover
		open={clockOpen}
		onOpenChange={onClockOpenChange}
		{date}
		{format}
		initialMode={clockInitialMode}
		anchor={clockAnchor}
		anchorPlacement={clockAnchorPlacement}
		onApply={(d) => {
			date = d;
			onChange?.(d);
		}}
	/>
</div>
