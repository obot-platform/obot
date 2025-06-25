<script lang="ts">
	import { responsive } from '$lib/stores';
	import { goto } from '$app/navigation';
	import { darkMode } from '$lib/stores';
	import { LoaderCircle, MenuIcon, X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { type PageProps } from './$types';
	import { browser } from '$app/environment';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import Footer from '$lib/components/Footer.svelte';
	import {
		sortByCreatedDate,
		sortByPreferredMcpOrder,
		sortTemplatesByFeaturedNameOrder
	} from '$lib/sort';
	import { ChatService, type MCP, type MCPInfo, type ProjectTemplate } from '$lib/services';
	import Logo from '$lib/components/navbar/Logo.svelte';
	import { q } from '$lib/url';
	import type { MCPCatalogEntry, MCPCatalogEntryServerManifest } from '$lib/services/admin/types';

	let { data }: PageProps = $props();
	let { authProviders, templates, loggedIn, isAdmin } = data;
	let loginDialog = $state<HTMLDialogElement>();
	let overrideRedirect = $state<string | null>(null);
	let signUp = $state(true);
	let fetchingMCPs = $state<Promise<MCPCatalogEntry[]>>();

	onMount(() => {
		if (browser && new URL(window.location.href).searchParams.get('rd') && !loggedIn) {
			loginDialog?.showModal();
		}

		if (!loggedIn) {
			fetchingMCPs = ChatService.listMCPs();
		} else {
			const redirectRoute = q('rd');
			if (redirectRoute) {
				goto(redirectRoute);
			}

			if (browser) {
				goto(isAdmin ? '/v2/admin/mcp-servers' : '/home');
			}
		}
	});

	const sortedTemplates = $derived(
		templates?.sort(sortByCreatedDate).sort(sortTemplatesByFeaturedNameOrder)
	);
	let rd = $derived.by(() => {
		if (browser) {
			const rd = new URL(window.location.href).searchParams.get('rd');
			if (rd) {
				return rd;
			}
		}
		if (overrideRedirect !== null) {
			return overrideRedirect;
		}
		return '/';
	});

	function closeLoginDialog() {
		loginDialog?.close();
		signUp = true;
	}
</script>

<svelte:head>
	<title>Obot - Build AI agents with MCP</title>
</svelte:head>

{#if !loggedIn}
	{@render unauthorizedContent()}
{:else}
	<div class="flex h-svh w-svw flex-col items-center justify-center">
		<div class="flex items-center justify-center">
			<div class="animate-bounce">
				<Logo />
			</div>
			<p class="text-base font-semibold">Logging in...</p>
		</div>
	</div>
{/if}

{#snippet unauthorizedContent()}
	<div class="relative w-full flex-col text-black dark:text-white">
		<!-- Header with logo and navigation -->
		<div class="colors-background sticky top-0 z-30 flex h-16 w-full items-center">
			<div class="relative flex items-end p-5">
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
			<div class="flex items-center gap-4 px-5">
				{#if !responsive.isMobile}
					{@render navLinks()}
				{/if}
				{#if !responsive.isMobile}
					<button
						class="icon-button"
						onclick={() => {
							loginDialog?.showModal();
						}}>Sign Up</button
					>
				{/if}
				<button
					class="button-primary py-1 text-sm"
					onclick={() => {
						signUp = false;
						loginDialog?.showModal();
					}}
				>
					Login
				</button>
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
			class="colors-background mx-auto flex w-full flex-col items-center justify-center gap-18 pb-6 md:gap-24 md:pb-12"
		>
			<div
				class="from-surface1 to-surface2 bg-surface1 flex w-full items-center justify-center bg-radial-[at_25%_25%] to-75%"
			>
				<div class="well my-8 flex w-full flex-col gap-4 md:max-w-(--breakpoint-2xl)">
					<div class="relative flex h-auto w-full flex-col md:flex-row">
						<div class="relative z-10 flex grow flex-col justify-center pr-8">
							<div class="flex flex-col">
								<h1 class="text-2xl font-bold md:text-3xl lg:text-5xl xl:text-6xl">
									Introducing Obot:
								</h1>
								<h1 class="text-2xl font-bold md:text-3xl lg:text-5xl xl:text-6xl">
									{#if responsive.isMobile}
										Build AI agents with MCP
									{:else}
										Build AI agents <br /> with MCP
									{/if}
								</h1>
							</div>
						</div>
						<div
							class="mt-8 flex w-full flex-shrink-0 grow rounded-xl bg-white p-4 shadow-md md:mt-0 md:w-[390px] lg:w-[600px] xl:w-[775px] dark:bg-black"
						>
							{#if darkMode.isDark}
								<img
									src="/landing/images/landing_dark.webp"
									alt="landing"
									class="rounded-xl object-contain"
								/>
							{:else}
								<img
									src="/landing/images/landing.webp"
									alt="landing"
									class="rounded-xl object-contain"
								/>
							{/if}
						</div>
					</div>
				</div>
			</div>
			<div
				class="well mb-12 flex w-full max-w-(--breakpoint-2xl) flex-col items-center overflow-hidden"
			>
				<div class="relative flex w-full max-w-(--breakpoint-xl) items-center justify-center">
					<div
						class="bg-surface3 absolute top-1/2 left-1/2 h-[1px] w-full -translate-x-1/2 -translate-y-1/2"
					></div>
					<h2 class="relative z-10 bg-white px-4 text-xl font-semibold dark:bg-black">
						Get Started
					</h2>
				</div>

				{#if responsive.isMobile}
					<div class="mt-12 flex w-full flex-col items-center justify-center gap-6">
						<div class="flex flex-col gap-3">
							<h3 class="self-center text-lg font-semibold">Agents</h3>
							{#each sortedTemplates.slice(0, 5) as project}
								{@render featuredAgentTemplateCard(project)}
							{/each}
							{@render browseAllAgents()}
						</div>
						<div class="flex flex-col gap-3">
							<h3 class="self-center text-lg font-semibold">MCP Servers</h3>
							{#await fetchingMCPs}
								<LoaderCircle class="size-6 animate-spin" />
							{:then mcps}
								{@const sortedMcps = mcps?.sort(sortByPreferredMcpOrder) ?? []}
								{#each sortedMcps.slice(0, 10) as mcp}
									{#if mcp.commandManifest}
										{@render featuredMcpCard(mcp.id, mcp.commandManifest)}
									{/if}
									{#if mcp.urlManifest}
										{@render featuredMcpCard(mcp.id, mcp.urlManifest)}
									{/if}
								{/each}
								{@render browseAllMcpServers()}
							{/await}
						</div>
					</div>
				{:else}
					<div class="flex w-full flex-col items-center justify-center">
						<div class="flex w-full max-w-(--breakpoint-xl) flex-wrap md:flex-nowrap">
							<div class=" flex flex-1 flex-col items-center gap-4 pt-12 pr-4">
								<h3 class="text-lg font-semibold">Agents</h3>
								<div class="border-surface2 flex flex-col items-center gap-3 border-r-2 pr-4">
									{#each sortedTemplates.slice(0, 5) as project}
										{@render featuredAgentTemplateCard(project)}
									{/each}
								</div>
								{@render browseAllAgents()}
							</div>
							<div class="flex flex-1 flex-col items-center gap-4 pt-12 lg:flex-2">
								<h3 class="flex w-full justify-center text-lg font-semibold">MCP Servers</h3>
								<div class="flex w-full gap-3">
									{#await fetchingMCPs}
										<LoaderCircle class="size-6 animate-spin" />
									{:then mcps}
										{@const sortedMcps = mcps?.sort(sortByPreferredMcpOrder) ?? []}
										<div class="flex flex-1 flex-col items-center gap-3">
											{#each sortedMcps.slice(0, 5) as mcp}
												{#if mcp.commandManifest}
													{@render featuredMcpCard(mcp.id, mcp.commandManifest)}
												{/if}
												{#if mcp.urlManifest}
													{@render featuredMcpCard(mcp.id, mcp.urlManifest)}
												{/if}
											{/each}
										</div>
										<div class="hidden flex-1 flex-col items-center gap-3 lg:flex">
											{#each sortedMcps.slice(5, 10) as mcp}
												{#if mcp.commandManifest}
													{@render featuredMcpCard(mcp.id, mcp.commandManifest)}
												{/if}
												{#if mcp.urlManifest}
													{@render featuredMcpCard(mcp.id, mcp.urlManifest)}
												{/if}
											{/each}
										</div>
									{/await}
								</div>
								{@render browseAllMcpServers()}
							</div>
						</div>
					</div>
				{/if}
			</div>
		</main>
		<Footer />

		<!-- Login Modal -->
		<dialog
			bind:this={loginDialog}
			use:clickOutside={closeLoginDialog}
			class="fixed top-1/2 left-1/2 m-0 h-fit max-h-none w-full max-w-none -translate-x-1/2 -translate-y-1/2 rounded-none p-4 shadow-lg backdrop:bg-black/50 md:max-h-fit md:max-w-md md:rounded-3xl"
		>
			<div class="flex w-full justify-end">
				<button type="button" class="icon-button" onclick={closeLoginDialog} aria-label="Close">
					<X size={24} />
				</button>
			</div>
			<div class="relative z-10 mb-6 flex w-full flex-col items-center justify-center gap-6">
				{#if darkMode.isDark}
					<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
				{:else}
					<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
				{/if}
				<p class="text-md px-8 text-center font-light text-gray-500 md:px-8 dark:text-gray-300">
					{#if signUp}
						You're almost there! Create an account and you'll be on our way to building and
						interacting with your own Obot agent.
					{:else}
						Welcome back! Log back in to start creating or interacting with your Obot agent again.
					{/if}
				</p>
				<h3 class="dark:bg-surface2 bg-white px-2 text-lg font-semibold">
					{signUp ? 'Sign Up With Obot' : 'Sign in to Your Account'}
				</h3>
			</div>

			<div
				class="border-surface3 relative -top-[18px] flex -translate-y-5 flex-col items-center gap-4 rounded-xl border-2 px-4 pt-6 pb-4"
			>
				{#each authProviders as provider}
					<a
						rel="external"
						href="/oauth2/start?rd={encodeURIComponent(
							overrideRedirect !== null ? overrideRedirect : rd
						)}&obot-auth-provider={provider.namespace}/{provider.id}"
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
{/snippet}

{#snippet navLinks()}
	<a
		href="https://docs.obot.ai"
		class={responsive.isMobile ? 'icon-button' : 'nav-link'}
		rel="external"
		target="_blank">Docs</a
	>
	<a
		href="https://discord.gg/9sSf4UyAMC"
		class={responsive.isMobile ? 'icon-button' : 'nav-link'}
		rel="external"
		target="_blank"
	>
		{#if darkMode.isDark}
			<img src="/user/images/discord-mark/discord-mark-white.svg" alt="Discord" class="h-6" />
		{:else}
			<img src="/user/images/discord-mark/discord-mark.svg" alt="Discord" class="h-6" />
		{/if}
	</a>
	<a
		href="https://github.com/obot-platform/obot"
		class={responsive.isMobile ? 'icon-button' : 'nav-link'}
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

{#snippet browseAllAgents()}
	<button
		onclick={() => {
			overrideRedirect = `/catalog?type=agents`;
			loginDialog?.showModal();
		}}
		class="button-text w-full text-center transition-colors duration-300 hover:text-inherit"
	>
		Browse All Agents
	</button>
{/snippet}

{#snippet browseAllMcpServers()}
	<button
		onclick={() => {
			overrideRedirect = `/catalog?type=mcps`;
			loginDialog?.showModal();
		}}
		class="button-text w-full text-center transition-colors duration-300 hover:text-inherit"
	>
		Browse All MCP Servers
	</button>
{/snippet}

{#snippet featuredAgentTemplateCard(template: ProjectTemplate)}
	<button
		class="bg-surface1 flex w-full items-center gap-3 rounded-xl p-3"
		onclick={() => {
			overrideRedirect = `/catalog?type=agents&id=${template.id}`;
			loginDialog?.showModal();
		}}
	>
		<div class="h-fit w-fit flex-shrink-0 rounded-md bg-gray-50 p-1 dark:bg-gray-600">
			<img src={template.projectSnapshot.icons?.icon} alt={template.name} class="size-6" />
		</div>
		<div class="flex flex-col text-left">
			<h4 class="line-clamp-1 text-sm font-semibold">{template.name}</h4>
			<p class="line-clamp-1 text-xs font-light">
				{template.projectSnapshot.description}
			</p>
		</div>
	</button>
{/snippet}

{#snippet featuredMcpCard(id: string, mcp: MCPCatalogEntryServerManifest)}
	<button
		class="bg-surface2 flex w-full items-center gap-3 rounded-xl p-3"
		onclick={() => {
			overrideRedirect = `/catalog?type=mcps&id=${id}`;
			loginDialog?.showModal();
		}}
	>
		<div class="h-fit w-fit flex-shrink-0 rounded-md bg-gray-50 p-1 dark:bg-gray-600">
			<img src={mcp.icon} alt={`${mcp.name} logo`} class="size-6" />
		</div>
		<div class="flex flex-col text-left">
			<h4 class="line-clamp-1 text-sm font-semibold">{mcp.name}</h4>
			<p class="line-clamp-1 text-xs font-light">
				{mcp.description}
			</p>
		</div>
	</button>
{/snippet}

<style lang="postcss">
	:global {
		.well {
			padding-left: 1rem;
			padding-right: 1rem;
			@media (min-width: 1024px) {
				padding-left: 4rem;
				padding-right: 4rem;
			}
			@media (min-width: 768px) {
				padding-left: 2rem;
				padding-right: 2rem;
			}
		}
	}
</style>
