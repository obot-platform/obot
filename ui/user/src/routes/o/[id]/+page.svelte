<script lang="ts">
	import New from '$lib/components/New.svelte';
	import { onMount } from 'svelte';
	import { assistants, darkMode } from '$lib/stores';
	import { page } from '$app/stores';
	import { get } from 'svelte/store';

	let dialog: ReturnType<typeof New>;

	onMount(async () => {
		await assistants.load();
		dialog?.show(get(page).params.id);
	});
</script>

<div class="flex size-full items-center justify-center p-20">
	{#if darkMode.isDark}
		<img src="/user/images/obot-logo-blue-white-text.svg" alt="Obot logo" class="h-96" />
	{:else}
		<img src="/user/images/obot-logo-blue-black-text.svg" alt="Obot logo" class="h-96" />
	{/if}
</div>
<New bind:this={dialog} closable={false} />
