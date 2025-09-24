<script lang="ts">
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	type Props = Record<string, unknown> & {
		class?: string;
		children?: Snippet<[{ x: boolean; y: boolean }]>;
	};

	let { class: klass = '', children, ...restProps }: Props = $props();

	let element: HTMLElement | null | undefined = $state();

	let clientWidth = $state(0);
	let x = $derived(element ? element.scrollWidth > clientWidth : false);

	let clientHeight = $state(0);
	let y = $derived(element ? element.scrollHeight > clientHeight : false);
</script>

<div
	bind:this={element}
	bind:clientWidth
	bind:clientHeight
	class={twMerge('flex w-full items-center', klass)}
	{...restProps}
>
	{@render children?.({
		x: x,
		y: y
	})}
</div>
