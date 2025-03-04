<script lang="ts">
	import { createTooltip } from '$lib/actions/tooltip.svelte';
	import type { Snippet } from 'svelte';
	import type { ClassValue } from 'svelte/elements';

	type Props = {
		children: Snippet;
		content: Snippet;
		class?: ClassValue;
		classes?: { tooltip?: ClassValue };
		disabled?: boolean;
	};

	let { children, content, class: className, classes = {}, disabled }: Props = $props();

	const tooltip = createTooltip({
		disabled: () => !!disabled,
		delay: 200,
		placement: 'top'
	});
</script>

<div
	use:tooltip.content
	class={['rounded-lg bg-blue-500 px-2 py-1 text-white dark:text-black', classes.tooltip]}
>
	{@render content()}
</div>

<div use:tooltip.anchor class={className}>
	{@render children()}
</div>
