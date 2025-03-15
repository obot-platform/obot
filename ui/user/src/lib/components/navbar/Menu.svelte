<script lang="ts">
	import { popover } from '$lib/actions';
	import type { Snippet } from 'svelte';
	import { RotateCw } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		classes?: {
			dialog?: string;
			button?: string;
		};
		onLoad?: () => void | Promise<void>;
		icon: Snippet;
		showRefresh?: boolean;
		show?: boolean;
		body: Snippet;
		header?: Snippet;
		title: string;
		description?: string;
		slide?: 'left' | 'up';
		fixed?: boolean;
	}

	let {
		onLoad,
		icon,
		header,
		body,
		title,
		description,
		show,
		classes,
		showRefresh = true,
		slide,
		fixed
	}: Props = $props();
	let loading = $state(false);
	const { ref, tooltip, toggle } = popover({
		placement: 'bottom',
		slide,
		fixed
	});

	$effect(() => {
		// this is mostly for development, easy way to show a menu to develop it
		if (show) {
			toggle(true);
		}
	});

	function load() {
		if (!onLoad) {
			return;
		}
		const ret = onLoad();
		if (ret instanceof Promise) {
			loading = true;
			const start = Date.now();
			ret.finally(() => {
				const delay = 1000 - (Date.now() - start);
				if (delay > 0) {
					setTimeout(() => {
						loading = false;
					}, delay);
				} else {
					loading = false;
				}
			});
		}
	}

	export { toggle };
</script>

<button
	use:ref
	class={twMerge('icon-button z-20', classes?.button)}
	onclick={() => {
		load();
		toggle();
	}}
	type="button"
>
	{@render icon()}
</button>

<div
	use:tooltip
	class={twMerge(
		'default-dialog z-30 flex w-screen flex-col divide-y divide-gray-200 p-6 dark:divide-gray-700 md:w-96',
		classes?.dialog
	)}
>
	<div class="mb-4">
		{#if header}
			{@render header()}
		{:else}
			<div class="flex justify-between">
				{title}
				{#if onLoad && showRefresh}
					<button onclick={load}>
						<RotateCw class="h-4 w-4 {loading ? 'animate-spin' : ''}" />
					</button>
				{/if}
			</div>
		{/if}
		{#if description}
			<p class="mt-1 text-xs font-normal text-gray-700 dark:text-gray-300">{description}</p>
		{/if}
	</div>
	{@render body()}
</div>
