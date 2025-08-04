<script lang="ts">
	import { addMinutes, getHours, getMinutes, setHours, setMinutes, subMinutes } from 'date-fns';
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

<div class="time-input bg-surface1/50 flex h-14 items-center gap-2 rounded-md">
	<div class="flex min-h-full flex-1 text-xl">
		<input
			class="min-h-full w-full bg-transparent px-4 text-end"
			type="number"
			max="12"
			min="0"
			value={hours % 12}
			onkeydown={(ev) => {
				if (['ArrowDown', 'ArrowUp'].includes(ev.key)) {
					ev.preventDefault();
				}

				if (
					(ev.key === 'Backspace' || ev.key === 'Delete') &&
					ev.currentTarget.value.length === 1
				) {
					// Reset to 0 if the input is cleared

					ev.preventDefault();

					const hours = 0;

					date = setHours(date, hours);
					onChange?.(date);

					ev.currentTarget.value = hours + '';
				}
			}}
			onkeyup={(ev) => {
				if (ev.key === 'ArrowDown') {
					ev.preventDefault();

					date = subMinutes(date, 60);

					onChange?.(date);
				}

				if (ev.key === 'ArrowUp') {
					ev.preventDefault();

					date = addMinutes(date, 60);
					onChange?.(date);
				}
			}}
			oninput={(ev) => {
				const valueAsNumber = ev.currentTarget.valueAsNumber;
				date = setHours(date, (valueAsNumber + amPmAsNumber * 12) % 24);
				onChange?.(date);
			}}
		/>
	</div>

	<div class="text-4xl font-bold">:</div>

	<div class=" flex min-h-full flex-1 rounded-md text-xl">
		<input
			class="min-h-full w-full bg-transparent px-4"
			type="number"
			max="60"
			min="0"
			value={minutes % 60}
			onkeydown={(ev) => {
				if (['ArrowDown', 'ArrowUp'].includes(ev.key)) {
					ev.preventDefault();
				}

				if (
					(ev.key === 'Backspace' || ev.key === 'Delete') &&
					ev.currentTarget.value.length === 1
				) {
					// Reset to 0 if the input is cleared

					ev.preventDefault();

					const minutes = 0;

					date = setMinutes(date, minutes);
					onChange?.(date);

					ev.currentTarget.value = minutes + '';
				}
			}}
			onkeyup={(ev) => {
				if (ev.key === 'ArrowDown') {
					date = subMinutes(date, 1);

					onChange?.(date);
				}

				if (ev.key === 'ArrowUp') {
					date = addMinutes(date, 1);

					onChange?.(date);
				}
			}}
			oninput={(ev) => {
				const valueAsNumber = ev.currentTarget.valueAsNumber;
				date = setMinutes(date, valueAsNumber % 60);

				onChange?.(date);
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
