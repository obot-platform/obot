<script lang="ts">
	import { profile, responsive } from '$lib/stores';
	import { goto } from '$app/navigation';
	import { darkMode } from '$lib/stores';
	import { MenuIcon, X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { type PageProps } from './$types';
	import { browser } from '$app/environment';
	import { type ProjectShare, type ToolReference } from '$lib/services';
	import ToolPill from '$lib/components/ToolPill.svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';

	let { data }: PageProps = $props();
	let { authProviders, assistants, assistantsLoaded, featuredProjectShares, tools } = data;
	let loginDialog = $state<HTMLDialogElement>();
	let projectShareRedirect = $state<string | null>(null);

	onMount(async () => {
		if (!assistantsLoaded) {
			show();
		}

		if (browser && new URL(window.location.href).searchParams.get('rd')) {
			loginDialog?.showModal();
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
				class="absolute top-0 left-0 h-full w-full object-cover opacity-85"
			/>
			<div
				class="to-surface1 absolute -bottom-0 left-0 z-10 h-2/4 w-full bg-linear-to-b from-transparent via-transparent transition-colors duration-300"
			></div>
		</div>
		<div class="flex h-full flex-col gap-2 px-4 py-2">
			<h4 class="font-semibold">{projectShare.name || 'Untitled'}</h4>
			<p class="text-gray line-clamp-3 text-xs">{projectShare.description}</p>
			{#if projectShare.tools}
				<div class="mt-auto flex flex-wrap items-center justify-end gap-2 py-2">
					{#each projectShare.tools.slice(0, 3) as tool}
						{@const toolData = tools.get(tool)}
						{#if toolData}
							<ToolPill tool={toolData} />
						{/if}
					{/each}
					{#if projectShare.tools.length > 3}
						<ToolPill
							tools={projectShare.tools
								.slice(3)
								.map((t) => tools.get(t))
								.filter((t): t is ToolReference => !!t)}
						/>
					{/if}
				</div>
			{:else}
				<div class="min-h-2"></div>
				<!-- placeholder -->
			{/if}
		</div>
	</a>
{/snippet}

{#snippet navLinks()}
	<a href="https://docs.obot.ai" class="icon-button" rel="external" target="_blank">Docs</a>
	<a href="https://discord.gg/9sSf4UyAMC" class="icon-button" rel="external" target="_blank">
		{#if darkMode.isDark}
			<img src="/user/images/discord-mark/discord-mark-white.svg" alt="Discord" class="h-6" />
		{:else}
			<img src="/user/images/discord-mark/discord-mark.svg" alt="Discord" class="h-6" />
		{/if}
	</a>
	<a
		href="https://github.com/obot-platform/obot"
		class="icon-button"
		rel="external"
		target="_blank"
	>
		{#if darkMode.isDark}
			<img src="/user/images/github-mark/github-mark-white.svg" alt="GitHub" class="h-6" />
		{:else}
			<img src="/user/images/github-mark/github-mark.svg" alt="GitHub" class="h-6" />
		{/if}
	</a>
{/snippet}

<svelte:head>
	<title>Obot - Do more with AI</title>
</svelte:head>

<div bind:this={div} class="relative hidden h-dvh w-full flex-col text-black dark:text-white">
	<!-- Header with logo and navigation -->
	<div class="colors-background flex h-16 w-full items-center p-5">
		<div class="relative flex items-end">
			{#if darkMode.isDark}
				<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
			{:else}
				<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
			{/if}
			<div class="ml-1.5 -translate-y-1">
				<span
					class="rounded-full border-2 border-blue-400 px-1.5 py-[1px] text-[10px] font-bold text-blue-400 dark:border-blue-400 dark:text-blue-400"
				>
					BETA
				</span>
			</div>
		</div>
		<div class="grow"></div>
		<div class="flex items-center gap-4">
			{#if !responsive.isMobile}
				{@render navLinks()}
			{/if}
			<button class="icon-button" onclick={() => loginDialog?.showModal()}>Login</button>
			{#if responsive.isMobile}
				<Menu
					slide="left"
					fixed
					classes={{
						dialog:
							'rounded-none h-[calc(100vh-64px)] p-4 left-0 top-[64px] w-full h-full px-4 divide-transparent dark:divide-transparent'
					}}
					title=""
				>
					{#snippet icon()}
						<MenuIcon />
					{/snippet}
					{#snippet body()}
						<div class="flex flex-col gap-2 py-2">
							{@render navLinks()}
						</div>
					{/snippet}
				</Menu>
			{/if}
		</div>
	</div>

	<main
		class="colors-background mx-auto flex w-full max-w-(--breakpoint-2xl) flex-col justify-center px-4 pb-12 md:px-12"
	>
		<div class="mt-16 mb-16 flex flex-col items-center text-center">
			<h1 class="text-2xl font-bold md:text-3xl">Do more with AI</h1>
			<p class="mt-4 max-w-full text-base md:max-w-2xl md:text-xl">
				Introducing Obot, a free platform for creating and sharing AI agents.
			</p>
		</div>

		{#if featuredProjectShares.length > 0}
			<div class="my-4 flex w-full flex-col gap-4 md:my-12">
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
		class="backdrop:bg-opacity-50 fixed top-1/2 left-1/2 m-0 h-dvh max-h-none w-full max-w-none -translate-x-1/2 -translate-y-1/2 rounded-none p-4 shadow-lg backdrop:bg-black md:max-h-fit md:max-w-md md:rounded-3xl"
	>
		<div class="flex w-full justify-end">
			<button
				type="button"
				class="icon-button"
				onclick={() => loginDialog?.close()}
				aria-label="Close"
			>
				<X size={24} />
			</button>
		</div>
		<div class="relative z-10 mb-6 flex w-full flex-col items-center justify-center gap-6">
			{#if darkMode.isDark}
				<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
			{:else}
				<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
			{/if}
			<p class="px-8 text-center text-sm font-light text-gray-500 md:px-12 dark:text-gray-300">
				You're almost there! Log in to start creating or interacting with your Obot.
			</p>
			<h3 class="dark:bg-surface2 bg-white px-2 text-lg font-semibold">Sign in to Your Account</h3>
		</div>

		<div
			class="border-surface3 relative -top-[18px] flex -translate-y-5 flex-col items-center gap-4 rounded-xl border-2 px-4 pt-6 pb-4"
		>
			{#each authProviders as provider}
				<a
					rel="external"
					href="/oauth2/start?rd={projectShareRedirect !== null
						? projectShareRedirect
						: rd}&obot-auth-provider={provider.namespace}/{provider.id}"
					class="group bg-surface1 hover:bg-surface2 dark:bg-surface1 dark:hover:bg-surface3 flex w-full items-center justify-center gap-1.5 rounded-full p-2 px-8 text-lg font-semibold"
					onclick={(e) => {
						console.log(`post-auth redirect ${e.target}`);
					}}
				>
					{#if provider.icon}
						<img
							class="h-6 w-6 rounded-full bg-transparent p-1 dark:bg-gray-600"
							src={provider.icon}
							alt={provider.name}
						/>
						<span class="text-center text-sm font-light">Continue with {provider.name}</span>
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
