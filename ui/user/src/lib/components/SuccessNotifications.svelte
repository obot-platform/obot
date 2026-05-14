<script lang="ts">
	import { navigating } from '$app/state';
	import { success } from '$lib/stores/success';
	import Success from './Success.svelte';

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

<div bind:this={div} class="toast toast-bottom toast-end z-50">
	{#each $success as message (message.id)}
		<Success message={message.message} onClose={() => success.remove(message.id)} />
	{/each}
</div>
