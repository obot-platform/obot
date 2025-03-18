<script lang="ts">
	import { onDestroy, type Snippet } from 'svelte';
	import { EllipsisVertical } from 'lucide-svelte';
	import { popover } from '$lib/actions';
	import type { Placement } from '@floating-ui/dom';
	import { responsive } from '$lib/stores';

	interface Props {
		children: Snippet;
		class?: string;
		placement?: Placement;
		icon?: Snippet;
		onClick?: () => void;
	}

	let {
		children,
		class: clazz = 'icon-button',
		placement = 'right-start',
		icon,
		onClick
	}: Props = $props();
	let tooltipEl: HTMLElement;
	let container: HTMLElement;

	const { tooltip, ref, toggle } = popover({
		placement
	});

	$effect(() => {
		if (responsive.isMobile && tooltipEl) {
			// Create container and move tooltip into it
			container = document.createElement('div');
			document.body.appendChild(container);
			container.appendChild(tooltipEl);

			return () => {
				// Clean up when mobile state changes or component destroys
				container?.remove();
			};
		}
	});

	onDestroy(() => {
		// Additional cleanup on component destruction
		container?.remove();
	});
</script>

<button
	class={clazz}
	use:ref
	onclick={(e) => {
		toggle();
		e.preventDefault();
		onClick?.();
	}}
>
	{#if icon}
		{@render icon()}
	{:else}
		<EllipsisVertical class="icon-default transition-colors duration-300" />
	{/if}
</button>
<div
	bind:this={tooltipEl}
	use:tooltip={{
		fixed: responsive.isMobile ? true : undefined,
		slide: responsive.isMobile ? 'up' : undefined
	}}
	role="none"
	onclick={(e) => {
		e.preventDefault();
		toggle();
	}}
	class={responsive.isMobile ? 'bottom-0 left-0 w-full' : ''}
>
	{@render children()}
</div>
