<script lang="ts">
	import { CircleX } from 'lucide-svelte/icons';
	import { X } from 'lucide-svelte/icons';
	import { errors, profile } from '$lib/stores';
	import { navigating } from '$app/state';

	let div: HTMLElement;

	$effect(() => {
		if (profile.current.loaded && div.classList.contains('hidden')) {
			div.classList.remove('hidden');
			div.classList.add('flex');
		}
	});

	$effect(() => {
		if (navigating) {
			errors.items = [];
		}
	});
</script>

<div bind:this={div} class="absolute right-0 bottom-0 z-50 hidden flex-col gap-2 pr-5 pb-5">
	{#each errors.items as error, i (i)}
		<div
			class="relative flex max-w-sm items-center gap-2 rounded-xl bg-gray-50 p-5 pr-12 dark:bg-gray-950"
		>
			<div>
				<CircleX class="h-5 w-5" />
			</div>
			<div class="line-clamp-3 pr-5 text-sm font-normal break-all">
				{error.message}
			</div>
			<button
				type="button"
				onclick={() => errors.items.splice(i, 1)}
				class="absolute top-0 right-0 p-4"
			>
				<X class="h-5 w-5" />
			</button>
		</div>
	{/each}
</div>
