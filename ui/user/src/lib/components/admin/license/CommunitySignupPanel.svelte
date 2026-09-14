<script lang="ts">
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		children: Snippet;
		class?: string;
		labelledBy?: string;
		as?: 'div' | 'aside' | 'section';
	}

	let { children, class: className, labelledBy, as = 'div' }: Props = $props();
</script>

<svelte:element
	this={as}
	class={twMerge(
		'relative overflow-hidden rounded-box border border-primary/20 bg-base-100 dark:bg-base-200 shadow-sm',
		className
	)}
	aria-labelledby={labelledBy}
>
	<div class="pointer-events-none absolute inset-0" aria-hidden="true">
		<div
			class="absolute inset-0 bg-linear-to-br from-primary/10 via-transparent to-primary/5"
		></div>
		<div class="absolute -top-12 -left-8 size-40 rounded-full border border-primary/15"></div>
		<div class="absolute -top-6 -left-2 size-24 rounded-full border border-primary/10"></div>
		<div class="absolute -right-16 -bottom-20 size-52 rounded-full bg-primary/5"></div>
		<div
			class="absolute inset-y-0 left-0 w-1/4 bg-linear-to-r from-primary/10 to-transparent"
		></div>
	</div>

	<div class="relative">
		{@render children()}
	</div>
</svelte:element>
