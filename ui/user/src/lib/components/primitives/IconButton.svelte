<script lang="ts">
	import { tooltip, type TooltipOptions } from '$lib/actions/tooltip.svelte';
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import { twMerge, type ClassNameValue } from 'tailwind-merge';

	interface Props extends HTMLButtonAttributes {
		children: Snippet;
		tooltip?: TooltipOptions;
		variant?: 'default' | 'danger' | 'danger2' | 'primary';
	}

	const defaultClasses = 'btn btn-square shrink-0 disabled:cursor-not-allowed disabled:opacity-50';
	let {
		children,
		class: className,
		tooltip: tooltipOptions,
		variant = 'default',
		...props
	}: Props = $props();

	const variantClasses = $derived(
		variant === 'danger2'
			? 'btn-error'
			: variant === 'primary'
				? 'btn-primary btn-soft'
				: variant === 'danger'
					? 'btn-ghost text-muted-content hover:text-error'
					: 'btn-ghost text-muted-content'
	);
</script>

{#if tooltipOptions}
	<button
		type="button"
		aria-label={tooltipOptions.text}
		class={twMerge(defaultClasses, variantClasses, className as ClassNameValue)}
		{...props}
		use:tooltip={{
			...tooltipOptions,
			classes: [...(tooltipOptions?.classes ?? []), 'z-50']
		}}
	>
		{@render children()}
	</button>
{:else}
	<button
		type="button"
		class={twMerge(defaultClasses, variantClasses, className as ClassNameValue)}
		{...props}
	>
		{@render children()}
	</button>
{/if}
