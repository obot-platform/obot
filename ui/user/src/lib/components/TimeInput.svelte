<script lang="ts">
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
	};

	let { date = $bindable(), onChange, class: klass }: Props = $props();

	const hours = $derived(getHours(date));
	const minutes = $derived(getMinutes(date));
	const isAm = $derived(hours < 12);
	const hour12 = $derived(hours % 12 === 0 ? 12 : hours % 12);

	let hourDraft = $state('');
	let minuteDraft = $state('');

	$effect(() => {
		hourDraft = hour12.toString().padStart(2, '0');
	});
	$effect(() => {
		minuteDraft = (minutes % 60).toString().padStart(2, '0');
	});

	function commitHour() {
		const trimmed = hourDraft.trim();
		if (trimmed === '') {
			hourDraft = hour12.toString().padStart(2, '0');
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
</script>

<div class={twMerge('time-input bg-surface1 flex h-14 items-center gap-2 rounded-md', klass)}>
	<div class="flex h-full flex-1 text-xl">
		<input
			class="w-[3ch] flex-1 bg-transparent px-4 text-end"
			type="text"
			inputmode="numeric"
			autocomplete="off"
			maxlength="2"
			bind:value={hourDraft}
			oninput={(ev) => {
				hourDraft = sanitizeHourDraft(ev.currentTarget.value);
			}}
			onblur={commitHour}
			onkeydown={(ev) => {
				if (['ArrowDown', 'ArrowUp'].includes(ev.key)) {
					ev.preventDefault();
					return;
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

	<div class="text-4xl font-bold">:</div>

	<div class="flex h-full flex-1 rounded-md text-xl">
		<input
			class="w-[3ch] flex-1 bg-transparent px-4"
			type="text"
			inputmode="numeric"
			autocomplete="off"
			maxlength="2"
			bind:value={minuteDraft}
			oninput={(ev) => {
				minuteDraft = sanitizeMinuteDraft(ev.currentTarget.value);
			}}
			onblur={commitMinute}
			onkeydown={(ev) => {
				if (['ArrowDown', 'ArrowUp'].includes(ev.key)) {
					ev.preventDefault();
					return;
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

	<div class="flex h-full flex-col gap-1 p-1 text-xs">
		<button
			class={twMerge(
				'bg-surface3/30 flex-1 rounded-sm px-1',
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
				'bg-surface3/30 flex-1 rounded-sm px-1',
				!isAm && 'text-primary bg-primary/10'
			)}
			onclick={() => {
				if (!isAm) return;
				date = setHours(date, (hours + 12) % 24);
				onChange?.(date);
			}}>PM</button
		>
	</div>
</div>
