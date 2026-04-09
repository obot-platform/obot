<script lang="ts">
	import { navigating } from '$app/state';
	import { errors, profile } from '$lib/stores';
	import { CircleX, Copy } from 'lucide-svelte/icons';
	import { X } from 'lucide-svelte/icons';
	import { fade } from 'svelte/transition';

	let div: HTMLElement;
	let copied = $state<Record<string, boolean>>({});

	$effect(() => {
		if (profile.current.loaded && div.classList.contains('hidden')) {
			div.classList.remove('hidden');
			div.classList.add('flex');
		}
	});

	$effect(() => {
		if (navigating) {
			errors.items = [];
			copied = {};
		}
	});
</script>

<div bind:this={div} class="absolute right-0 bottom-0 z-50 hidden flex-col gap-2 pr-5 pb-5">
	{#each errors.items as error, i (i)}
		<div
			class="relative flex max-w-sm flex-col gap-1 rounded-xl bg-gray-50 p-5 pr-12 dark:bg-gray-950"
		>
			<button
				class="group flex w-full items-center gap-2 text-left"
				onclick={() => {
					if (!navigator.clipboard) return;

					navigator.clipboard.writeText(error.message);
					copied[error.message] = true;
					setTimeout(() => {
						delete copied[error.message];
					}, 1000);
				}}
			>
				<div>
					<CircleX class="block size-5 group-hover:hidden" />
					<Copy class="hidden size-5 group-hover:block" />
				</div>
				<div class="line-clamp-3 pr-5 text-sm font-normal break-all">
					{error.message}
				</div>
			</button>
			{#if copied[error.message]}
				<div class="text-on-surface1 self-end text-xs" in:fade={{ duration: 200 }}>
					Error copied to clipboard.
				</div>
			{/if}
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
