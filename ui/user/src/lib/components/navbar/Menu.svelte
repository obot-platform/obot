<script lang="ts">
	import { popover } from '$lib/actions';
	import type { Snippet } from 'svelte';
	import { RotateCw } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		classes?: {
			button?: string;
		};
		onLoad?: () => void | Promise<void>;
		icon: Snippet;
		show?: boolean;
		body: Snippet;
		title: string;
		description?: string;
	}

	let { onLoad, icon, body, title, description, show, classes }: Props = $props();
	let loading = $state(false);
	const { ref, tooltip, toggle } = popover({
		placement: 'bottom',
		offset: 0
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
		loading = true;
		const start = Date.now();
		const ret = onLoad();
		if (ret instanceof Promise) {
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

<div use:tooltip class="z-30 w-screen px-2 md:w-96">
	<div
		class="default-dialog flex w-full flex-col divide-y divide-gray-200 p-6 dark:divide-gray-700"
	>
		<div class="mb-4">
			<div class="flex justify-between">
				{title}
				{#if onLoad}
					<button onclick={load}>
						<RotateCw class="h-4 w-4 {loading ? 'animate-spin' : ''}" />
					</button>
				{/if}
			</div>
			{#if description}
				<p class="mt-1 text-xs font-normal text-gray-700 dark:text-gray-300">{description}</p>
			{/if}
		</div>
		{@render body()}
	</div>
</div>
