<script lang="ts">
	import { profile } from '$lib/stores';
	import { goto } from '$app/navigation';
	import DarkModeToggle from '$lib/components/navbar/DarkModeToggle.svelte';
	import { darkMode } from '$lib/stores';
	import { X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { type PageProps } from './$types';
	import { browser } from '$app/environment';
	import { type ProjectShare } from '$lib/services';
	import { twMerge } from 'tailwind-merge';

	let { data }: PageProps = $props();
	let { authProviders, assistants, assistantsLoaded, featuredProjectShares, tools } = data;
	let loginDialog = $state<HTMLDialogElement>();
	let projectShareRedirect = $state<string | null>(null);

	onMount(async () => {
		if (!assistantsLoaded) {
			show();
		}
	});

	let div: HTMLElement;
	let rd = $derived.by(() => {
		if (browser) {
			const rd = new URL(window.location.href).searchParams.get('rd');
			if (rd) {
				return rd;
			}
		}
		if (projectShareRedirect !== null) {
			return projectShareRedirect;
		}
		return '/';
	});

	$effect(() => {
		let a = assistants.find((assistant) => assistant.default);
		if (a || assistants.length === 1) {
			goto(`/home`, { replaceState: true });
		} else if (assistantsLoaded) {
			window.location.href = '/admin/';
		}
	});

	$effect(() => {
		if (profile.current.unauthorized) {
			show();
		}
	});

	function show() {
		div.classList.remove('hidden');
		div.classList.add('flex');
	}

	function getImage(projectShare: ProjectShare) {
		const imageUrl = darkMode.isDark
			? projectShare.icons?.iconDark || projectShare.icons?.icon
			: projectShare.icons?.icon;

		return imageUrl ?? '/agent/images/placeholder.webp';
	}
</script>

{#snippet featuredProjectCard(projectShare: ProjectShare)}
	<a
		href="/"
		data-sveltekit-preload-data="off"
		class="card relative z-20 flex-col overflow-hidden shadow-md"
		onclick={(e) => {
			e.preventDefault();
			// Set the login redirect to the project's share URL
			if (browser) {
				projectShareRedirect = `/s/${projectShare.publicID}`;
				loginDialog?.showModal();
			}
		}}
	>
		<div class="relative aspect-video">
			<img
				alt={projectShare.name || 'Obot'}
				src={getImage(projectShare)}
				class="absolute left-0 top-0 h-full w-full object-cover opacity-85"
			/>
			<div
				class="absolute -bottom-0 left-0 z-10 h-2/4 w-full bg-gradient-to-b from-transparent via-transparent to-surface1 transition-colors duration-300"
			></div>
		</div>
		<div class="flex h-full flex-col gap-2 px-4 py-2">
			<h4 class="font-semibold">{projectShare.name || 'Untitled'}</h4>
			<p class="line-clamp-3 text-xs text-gray">{projectShare.description}</p>

			{#if projectShare.tools}
				<div class="mt-auto flex flex-wrap items-center justify-end gap-2">
					{#each projectShare.tools as tool}
						{@const toolData = tools.get(tool)}
						<div
							class="flex w-fit items-center gap-1 rounded-2xl bg-surface2 p-2 transition-all duration-300"
						>
							{#if toolData?.metadata?.icon}
								<img
									alt={toolData.name || 'Unknown'}
									src={toolData.metadata.icon}
									class={twMerge('h-4 w-4')}
								/>
							{/if}
						</div>
					{/each}
				</div>
			{:else}
				<div class="min-h-2"></div>
			{/if}
		</div>
	</a>
{/snippet}

<svelte:head>
	<title>Obot - Do more with AI</title>
</svelte:head>

<div bind:this={div} class="relative hidden h-dvh w-full flex-col text-black dark:text-white">
	<!-- Header with logo and navigation -->
	<div class="colors-background flex h-16 w-full items-center p-5">
		{#if darkMode.isDark}
			<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
		{:else}
			<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
		{/if}
		<div class="grow"></div>
		<div class="flex items-center gap-4">
			<a href="https://docs.obot.ai" class="icon-button" rel="external"> Docs </a>
			<a href="https://discord.gg/obot" class="icon-button" rel="external">
				{#if darkMode.isDark}
					<img src="/user/images/discord-mark/discord-mark-white.svg" alt="Discord" class="h-6" />
				{:else}
					<img src="/user/images/discord-mark/discord-mark.svg" alt="Discord" class="h-6" />
				{/if}
			</a>
			<a href="https://github.com/obot-platform/obot" class="icon-button">
				{#if darkMode.isDark}
					<img src="/user/images/github-mark/github-mark-white.svg" alt="GitHub" class="h-6" />
				{:else}
					<img src="/user/images/github-mark/github-mark.svg" alt="GitHub" class="h-6" />
				{/if}
			</a>
			<DarkModeToggle />
			<button class="icon-button" onclick={() => loginDialog?.showModal()}> Login </button>
		</div>
	</div>

	<main
		class="colors-background mx-auto flex w-full max-w-screen-2xl flex-col justify-center px-4 pb-12 md:px-12"
	>
		<div class="mb-16 mt-16 flex flex-col items-center text-center">
			<h1 class="text-4xl font-bold md:text-5xl">Do more with AI</h1>
			<p class="mt-4 max-w-2xl text-xl">
				Introducing Obot, a free platform for creating and sharing AI agents.
			</p>
		</div>

		{#if featuredProjectShares.length > 0}
			<div class="mb-12 mt-12 flex w-full flex-col gap-4">
				<div class="featured-card-layout">
					{#each featuredProjectShares as projectShare}
						{@render featuredProjectCard(projectShare)}
					{/each}
				</div>
			</div>
		{/if}
	</main>

	<!-- Login Modal -->
	<dialog
		bind:this={loginDialog}
		class="colors-surface2 w-full max-w-md rounded-3xl p-6 shadow-lg backdrop:bg-black backdrop:bg-opacity-50"
	>
		<div class="mb-6 flex items-center justify-between">
			<h3 class="text-xl font-semibold">Login to Obot</h3>
			<button
				type="button"
				class="icon-button"
				onclick={() => loginDialog?.close()}
				aria-label="Close"
			>
				<X size={24} />
			</button>
		</div>

		<div class="flex flex-col items-center gap-4">
			{#each authProviders as provider}
				<a
					rel="external"
					href="/oauth2/start?rd={projectShareRedirect !== null
						? projectShareRedirect
						: rd}&obot-auth-provider={provider.namespace}/{provider.id}"
					class="group flex w-full items-center justify-center gap-1 rounded-full bg-black p-2 px-8 text-lg font-semibold text-white dark:bg-white dark:text-black"
					onclick={(e) => {
						console.log(`post-auth redirect ${e.target}`);
					}}
				>
					{#if provider.icon}
						<img
							class="h-6 w-6 rounded-full p-1 group-hover:bg-white"
							src={provider.icon}
							alt={provider.name}
						/>
						<span class="flex-grow text-center">Login with {provider.name}</span>
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
	</dialog>
</div>

<style>
	.featured-card-layout {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
		gap: 1.5rem;
		margin: 0 auto;
		max-width: 1200px;
	}
</style>
