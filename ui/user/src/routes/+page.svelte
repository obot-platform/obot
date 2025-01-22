<script lang="ts">
	import { assistants } from '$lib/stores';
	import { profile } from '$lib/stores';
	import { goto } from '$app/navigation';
	import DarkModeToggle from '$lib/components/navbar/DarkModeToggle.svelte';
	import { darkMode } from '$lib/stores';
	import { Book } from '$lib/icons';
	import { loadedAssistants } from '$lib/stores';
	import { listAuthProviders, type AuthProvider } from '$lib/auth';
	import { onMount } from 'svelte';

	let authProviders: AuthProvider[] = $state([]);

	onMount(async () => {
		authProviders = await listAuthProviders();
	});

	let div: HTMLElement;

	$effect(() => {
		let id = $assistants.find((assistant) => assistant.default)?.id;
		if (!id) {
			id = $assistants.find((assistant) => assistant.id !== '')?.id;
		}
		if (id) {
			goto(`/${id}`);
		} else if ($loadedAssistants) {
			window.location.href = '/admin/';
		}
	});

	$effect(() => {
		if ($profile.unauthorized) {
			div.classList.remove('hidden');
			div.classList.add('flex');
		}
	});
</script>

<div bind:this={div} class="relative hidden h-dvh w-full items-center text-black dark:text-white">
	<div
		class="absolute right-0 top-0 flex items-center gap-4 p-4 pr-6 text-white hover:text-blue-50"
	>
		<DarkModeToggle />
		<a href="https://docs.obot.ai" class="icon-button" rel="external">
			<Book />
		</a>
		<a
			href="https://github.com/obot-platform/obot"
			class="icon-button text-white hover:text-blue-50"
		>
			{#if $darkMode}
				<img src="/user/images/github-mark/github-mark-white.svg" alt="GitHub" class="h-8" />
			{:else}
				<img src="/user/images/github-mark/github-mark.svg" alt="GitHub" class="h-8" />
			{/if}
		</a>
	</div>
	<div class="mx-auto flex flex-col items-center gap-16">
		<div class="flex items-end gap-4">
			{#if $darkMode}
				<img src="/user/images/obot-logo-blue-white-text.svg" alt="obot icon" class="h-64 px-5" />
			{:else}
				<img src="/user/images/obot-logo-blue-black-text.svg" alt="obot icon" class="h-64 px-5" />
			{/if}
		</div>

		<div class="mt-32 flex flex-col items-center gap-4">
			{#each authProviders as provider}
				<a
					onclick={() => {
						window.location.href =
							'/oauth2/start?rd=' +
							window.location.pathname +
							'&obot-auth-provider=' +
							provider.namespace +
							'/' +
							provider.id;
					}}
					rel="external"
					href={`/oauth2/start?obot-auth-provider=${provider.namespace}/${provider.id}&rd=/`}
					class="group flex items-center gap-1 rounded-full bg-black p-2 px-8 text-lg font-semibold text-white dark:bg-white dark:text-black"
				>
					{#if provider.icon}
						<img
							class="ml-2 h-6 w-6 rounded-full p-1 group-hover:bg-white"
							src={provider.icon}
							alt={provider.name}
						/>
						Login with {provider.name}
					{/if}
				</a>
			{/each}
			{#if authProviders.length === 0}
				<p>
					No auth providers configured. Please configure at least one auth provider in the admin
					panel.
				</p>
			{/if}
		</div>
	</div>
</div>
