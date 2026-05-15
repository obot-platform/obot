<script lang="ts">
	import { twMerge } from 'tailwind-merge';

	interface Props {
		/** Hue in degrees along the visible spectrum (0–360). */
		hue?: number;
		id?: string;
		disabled?: boolean;
		/** Accessible name when no visible label is tied via `for`. */
		'aria-label'?: string;
		class?: string;
	}

	let {
		hue = $bindable(0),
		id,
		disabled = false,
		'aria-label': ariaLabel = 'Spectrum color',
		class: className
	}: Props = $props();
</script>

<div class={twMerge('spectrum-slider relative flex w-full min-w-48 items-center py-1', className)}>
	<!-- Track: continuous hue gradient (ROYGBIV via HSL hues) -->
	<div
		class="pointer-events-none absolute inset-x-0 top-1/2 h-2 -translate-y-1/2 rounded-full border border-black/10 shadow-[inset_0_1px_2px_rgba(0,0,0,0.06)] dark:border-white/15 dark:shadow-[inset_0_1px_2px_rgba(0,0,0,0.25)]"
		style="background: linear-gradient(
			to right,
			hsl(0, 100%, 50%),
			hsl(60, 100%, 50%),
			hsl(120, 100%, 50%),
			hsl(180, 100%, 50%),
			hsl(240, 100%, 50%),
			hsl(300, 100%, 50%),
			hsl(360, 100%, 50%)
		);"
		aria-hidden="true"
	></div>

	<input
		{id}
		type="range"
		class="spectrum-slider-input relative z-10 h-8 w-full cursor-pointer bg-transparent disabled:cursor-not-allowed disabled:opacity-40"
		min="0"
		max="360"
		step="1"
		bind:value={hue}
		{disabled}
		aria-label={ariaLabel}
		aria-valuemin={0}
		aria-valuemax={360}
		aria-valuenow={hue}
	/>
</div>

<style lang="postcss">
	/* Thumb: ring only so the spectrum shows through the center (matches ROYGBIV reference). */
	.spectrum-slider-input {
		-webkit-appearance: none;
		appearance: none;
	}

	.spectrum-slider-input:focus {
		outline: none;
	}

	.spectrum-slider-input:focus-visible {
		outline: 2px solid var(--color-primary);
		outline-offset: 4px;
		border-radius: 9999px;
	}

	.spectrum-slider-input::-webkit-slider-runnable-track {
		height: 0.5rem;
		background: transparent;
		border-radius: 9999px;
	}

	.spectrum-slider-input::-webkit-slider-thumb {
		-webkit-appearance: none;
		appearance: none;
		width: 1.25rem;
		height: 1.25rem;
		margin-top: -0.375rem;
		border-radius: 9999px;
		border: 3px solid white;
		box-shadow:
			0 0 0 1px rgb(0 0 0 / 0.25),
			0 1px 3px rgb(0 0 0 / 0.2);
		background: transparent;
	}

	.spectrum-slider-input::-moz-range-track {
		height: 0.5rem;
		background: transparent;
		border-radius: 9999px;
	}

	.spectrum-slider-input::-moz-range-thumb {
		width: 1.25rem;
		height: 1.25rem;
		border: 3px solid white;
		border-radius: 9999px;
		box-shadow:
			0 0 0 1px rgb(0 0 0 / 0.25),
			0 1px 3px rgb(0 0 0 / 0.2);
		background: transparent;
	}

	.spectrum-slider-input:disabled::-webkit-slider-thumb {
		box-shadow: 0 0 0 1px rgb(0 0 0 / 0.15);
	}

	.spectrum-slider-input:disabled::-moz-range-thumb {
		box-shadow: 0 0 0 1px rgb(0 0 0 / 0.15);
	}
</style>
