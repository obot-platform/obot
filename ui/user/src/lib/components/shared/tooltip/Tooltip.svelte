<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
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

	let anchor = $state<HTMLElement>();
</script>

<div
	use:tooltip={{
		anchor,
		placement: 'top',
		delay: 200,
		get disabled() {
			return disabled;
		}
	}}
	class={['rounded-lg bg-blue-500 px-2 py-1 text-white dark:text-black', classes.tooltip]}
>
	{@render content()}
</div>

<div bind:this={anchor} class={className}>
	{@render children()}
</div>
