<script lang="ts">
	import { type Snippet } from 'svelte';
	import { EllipsisVertical } from 'lucide-svelte';
	import { popover } from '$lib/actions';
	import type { Placement } from '@floating-ui/dom';
	import { responsive } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		children: Snippet<[{ toggle: (newOpenValue?: boolean) => void }]>;
		class?: string;
		classes?: {
			menu?: string;
		};
		placement?: Placement;
		icon?: Snippet;
		onClick?: () => void;
		disablePortal?: boolean;
		el?: Element;
	}

	let {
		children,
		class: clazz = 'icon-button',
		classes,
		placement = 'right-start',
		icon,
		onClick,
		disablePortal,
		el
	}: Props = $props();

	const { tooltip, ref, toggle } = popover({
		get placement() {
			return placement;
		}
	});
</script>

<button
	class={clazz}
	use:ref
	onclick={(e) => {
		toggle();
		e.stopPropagation();
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
	class={twMerge(responsive.isMobile ? 'bottom-0 left-0 w-full' : '', classes?.menu)}
>
	{@render children({ toggle })}
</div>
