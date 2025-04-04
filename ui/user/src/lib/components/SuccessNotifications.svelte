<script lang="ts">
	import Success from './Success.svelte';
	import { success } from '$lib/stores/success';
	import { navigating } from '$app/state';

	let div: HTMLElement;

	$effect(() => {
		if (div.classList.contains('hidden')) {
			div.classList.remove('hidden');
			div.classList.add('flex');
		}
	});

	$effect(() => {
		if (navigating) {
			success.remove(0);
		}
	});
</script>

<div bind:this={div} class="absolute right-0 bottom-0 z-50 hidden flex-col gap-2 pr-5 pb-5">
	{#each $success as message}
		<Success message={message.message} onClose={() => success.remove(message.id)} />
	{/each}
</div>
