<script lang="ts">
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	export type ScrollState = {
		hasMoreLeft: boolean;
		hasMoreRight: boolean;
		scrollLeft: () => void;
		scrollRight: () => void;
	};

	type Props = Record<string, unknown> & {
		class?: string;
		children?: Snippet<[{ x: boolean; y: boolean } & ScrollState]>;
		scrollStep?: number;
	};

	let { class: klass = '', children, scrollStep = 200, ...restProps }: Props = $props();

	let element: HTMLElement | null | undefined = $state();

	let clientWidth = $state(0);
	let x = $derived(element ? element.scrollWidth > clientWidth : false);

	let clientHeight = $state(0);
	let y = $derived(element ? element.scrollHeight > clientHeight : false);

	let hasMoreLeft = $state(false);
	let hasMoreRight = $state(false);

	function checkScrollPosition() {
		if (!element) return;

		const { scrollLeft, scrollWidth, clientWidth } = element;
		hasMoreLeft = scrollLeft > 0;
		hasMoreRight = scrollLeft < scrollWidth - clientWidth - 1; // -1 for rounding errors
	}

	function scrollLeft() {
		element?.scrollBy({ left: -scrollStep, behavior: 'smooth' });
	}

	function scrollRight() {
		element?.scrollBy({ left: scrollStep, behavior: 'smooth' });
	}

	$effect(() => {
		void clientWidth;
		void x;
		checkScrollPosition();
	});

	$effect(() => {
		const el = element;
		if (!el) return;

		checkScrollPosition();
		el.addEventListener('scroll', checkScrollPosition);
		window.addEventListener('resize', checkScrollPosition);

		return () => {
			el.removeEventListener('scroll', checkScrollPosition);
			window.removeEventListener('resize', checkScrollPosition);
		};
	});
</script>

<div
	bind:this={element}
	bind:clientWidth
	bind:clientHeight
	class={twMerge('flex w-full items-center', klass)}
	{...restProps}
>
	{@render children?.({
		x,
		y,
		hasMoreLeft,
		hasMoreRight,
		scrollLeft,
		scrollRight
	})}
</div>
