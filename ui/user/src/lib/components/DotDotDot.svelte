<script lang="ts">
	import type { Snippet } from 'svelte';
	import { EllipsisVertical } from 'lucide-svelte';
	import { popover } from '$lib/actions';
	import type { Placement } from '@floating-ui/dom';

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
	const { tooltip, ref, toggle } = popover({
		placement
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
	use:tooltip
	role="none"
	onclick={(e) => {
		e.preventDefault();
		toggle();
	}}
>
	{@render children()}
</div>
