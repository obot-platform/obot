<script lang="ts">
	import highlight from 'highlight.js';
	import { assistants } from '$lib/stores';
	import { profile } from '$lib/stores';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import DarkModeToggle from '$lib/components/navbar/DarkModeToggle.svelte';
	import { darkMode } from '$lib/stores';
	import { Book } from '$lib/icons';

	onMount(() => {
		highlight.highlightAll();
	});

	let div: HTMLElement;

	$effect(() => {
		let id = $assistants.find((assistant) => assistant.id === 'otto')?.id;
		if (!id) {
			id = $assistants.find((assistant) => assistant.id !== '')?.id;
		}
		if (id) {
			goto(`/${id}`);
		}
	});

	$effect(() => {
		if ($profile.unauthorized) {
			div.classList.remove('hidden');
			div.classList.add('flex');
		}
	});
</script>

<div
	bind:this={div}
	class="relative hidden h-screen w-full items-center text-black dark:text-white"
>
	<div
		class="absolute right-0 top-0 flex items-center gap-4 p-4 pr-6 text-white hover:text-blue-50"
	>
		<DarkModeToggle />
		<a href="https://docs.otto8.ai" class="icon-button" rel="external">
			<Book />
		</a>
		<a href="https://github.com/obot-platform/obot" class="icon-button text-white hover:text-blue-50">
			{#if $darkMode}
				<img src="/images/github-mark/github-mark-white.svg" alt="GitHub" class="h-8" />
			{:else}
				<img src="/images/github-mark/github-mark.svg" alt="GitHub" class="h-8" />
			{/if}
		</a>
	</div>
	<div class="mx-auto flex flex-col items-center gap-8">
		<div class="flex items-end gap-4">
			{#if $darkMode}
				<img src="/images/otto8-logo-blue-white-text.svg" alt="otto icon" class="h-64 pb-4" />
			{:else}
				<img src="/images/otto8-logo-blue-black-text.svg" alt="otto icon" class="h-64 pb-4" />
			{/if}
		</div>
		<h1 class="text-7xl text-blue-50">
			<span class="text-black dark:text-white">Friendly. Open Source. Assistant.</span>
		</h1>

		<div class="mt-32 flex items-center gap-4">
			<a
				onclick={() => {
					window.location.href = '/oauth2/start?rd=' + window.location.pathname;
				}}
				rel="external"
				href="/oauth2/start?rd=/"
				class="group flex items-center gap-1 rounded-full bg-black p-2 px-8 text-lg font-semibold text-white dark:bg-white dark:text-black"
			>
				Login
				<img
					class="ml-2 h-6 w-6 rounded-full p-1 group-hover:bg-white"
					src="https://lh3.googleusercontent.com/COxitqgJr1sJnIDe8-jiKhxDx1FrYbtRHKJ9z_hELisAlapwE9LUPh6fcXIfb5vwpbMl4xl9H9TRFPc5NOO8Sb3VSgIBrfRYvW6cUA"
					alt="Google"
				/>
			</a>
		</div>
	</div>
</div>
