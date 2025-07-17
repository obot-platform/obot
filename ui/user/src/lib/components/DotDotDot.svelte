<script lang="ts">
	import { type Snippet } from 'svelte';
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
		disablePortal?: boolean;
		el?: Element;
	}

	let {
		children,
		class: clazz = 'icon-button',
		placement = 'right-start',
		icon,
		onClick,
		disablePortal,
		el
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
	use:tooltip={{
		fixed: responsive.isMobile ? true : undefined,
		slide: responsive.isMobile ? 'up' : undefined,
		disablePortal,
		el
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
