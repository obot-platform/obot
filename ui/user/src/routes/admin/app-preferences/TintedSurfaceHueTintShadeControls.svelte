<script lang="ts">
	import { SHADE_TICK_MAX, SHADE_TICK_NEUTRAL } from '$lib/colors';
	import SpectrumSlider from '$lib/components/SpectrumSlider.svelte';

	let {
		hue = $bindable(0),
		tint = $bindable(0),
		shade = $bindable(SHADE_TICK_NEUTRAL),
		hueAriaLabel = 'Surface hue'
	}: {
		hue?: number;
		tint?: number;
		shade?: number;
		hueAriaLabel?: string;
	} = $props();
</script>

<div class="flex items-center justify-between gap-2">
	<p class="text-sm font-light w-20 shrink-0">Hue</p>
	<SpectrumSlider bind:hue aria-label={hueAriaLabel} class="min-w-0 grow" />
</div>

<div class="flex items-center justify-between gap-2 mb-2">
	<p class="text-sm font-light w-20 shrink-0">Tint</p>
	<input
		type="range"
		min="0"
		max="100"
		bind:value={tint}
		class="range grow"
		aria-valuetext={`${tint}%`}
	/>
</div>

<div class="flex flex-col gap-1">
	<div class="flex items-center justify-between gap-2">
		<p class="text-sm font-light w-20 shrink-0">Shade</p>
		<div class="flex grow items-center gap-2 min-w-0 justify-end">
			<input
				type="range"
				min="0"
				max={SHADE_TICK_MAX}
				step="1"
				bind:value={shade}
				class="range grow"
				aria-valuemin={0}
				aria-valuemax={SHADE_TICK_MAX}
				aria-valuenow={shade}
				aria-valuetext={shade === SHADE_TICK_NEUTRAL
					? 'Balanced'
					: shade < SHADE_TICK_NEUTRAL
						? 'Darker'
						: 'Lighter'}
			/>
			<span class="text-xs font-light tabular-nums text-base-content/60 w-4 shrink-0 text-right"
				>{shade}</span
			>
		</div>
	</div>
</div>
