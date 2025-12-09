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
	};

	let { date = $bindable(), onChange }: Props = $props();

	const hours = $derived(getHours(date));
	const minutes = $derived(getMinutes(date));
	const isAm = $derived(hours < 12);
	const amPmAsNumber = $derived(+!isAm);
</script>

<div class="time-input bg-surface1 flex h-14 items-center gap-2 rounded-md">
	<div class="flex h-full flex-1 text-xl">
		<input
			class="w-[3ch] flex-1 bg-transparent px-4 text-end"
			type="number"
			max="12"
			min="0"
			bind:value={
				() => (hours % 12).toString().padStart(2, '0'),
				(v) => {
					const valueAsNumber = parseInt(v, 10) || 0;

					date = setHours(date, Math.min(valueAsNumber + amPmAsNumber * 12, 23));
					onChange?.(date);
				}
			}
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

	<div class=" flex h-full flex-1 rounded-md text-xl">
		<input
			class="w-[3ch] flex-1 bg-transparent px-4"
			type="number"
			max="60"
			min="0"
			bind:value={
				() => (minutes % 60).toString().padStart(2, '0'),
				(v) => {
					const valueAsNumber = parseInt(v, 10) || 0;

					date = setMinutes(date, Math.min(valueAsNumber, 59));
					onChange?.(date);
				}
			}
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

<style>
	/* For WebKit-based browsers (Chrome, Safari, Edge, Opera) */
	input::-webkit-outer-spin-button,
	input::-webkit-inner-spin-button {
		-webkit-appearance: none; /* Removes the default appearance */
		margin: 0; /* Removes any default margin */
	}

	/* For Mozilla Firefox */
	input[type='number'] {
		appearance: textfield; /* Standard property for compatibility */
		-moz-appearance: textfield; /* Hides the spin buttons in Firefox */
	}
</style>
