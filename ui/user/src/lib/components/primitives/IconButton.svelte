<script lang="ts">
	import { tooltip, type TooltipOptions } from '$lib/actions/tooltip.svelte';
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import { twMerge, type ClassNameValue } from 'tailwind-merge';

	interface Props extends HTMLButtonAttributes {
		children: Snippet;
		tooltip?: TooltipOptions;
		variant?: 'default' | 'error' | 'danger' | 'primary';
	}

	const defaultClasses =
		'btn btn-ghost btn-square shrink-0 disabled:cursor-not-allowed disabled:opacity-50';
	let {
		children,
		class: className,
		tooltip: tooltipOptions,
		variant = 'default',
		...props
	}: Props = $props();
</script>

{#if tooltipOptions}
	<button
		type="button"
		aria-label={tooltipOptions.text}
		class={twMerge(
			defaultClasses,
			variant === 'error'
				? 'btn-error'
				: variant === 'primary'
					? 'btn-primary btn-soft'
					: variant === 'danger'
						? 'hover:text-error'
						: '',
			className as ClassNameValue
		)}
		{...props}
		use:tooltip={tooltipOptions}
	>
		{@render children()}
	</button>
{:else}
	<button type="button" class={twMerge(defaultClasses, className as ClassNameValue)} {...props}>
		{@render children()}
	</button>
{/if}
