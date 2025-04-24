<script lang="ts">
	import { profile, responsive } from '$lib/stores';
	import { goto } from '$app/navigation';
	import { darkMode } from '$lib/stores';
	import { CalendarDays, ChevronsRight, MenuIcon, X } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { type PageProps } from './$types';
	import { browser } from '$app/environment';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { clickOutside } from '$lib/actions/clickoutside';
	import Footer from '$lib/components/Footer.svelte';
	import FeaturedMcpCard from '$lib/components/mcp/FeaturedMcpCard.svelte';
	import { sortByPreferredMcpOrder } from '$lib/sort';

	let { data }: PageProps = $props();
	let { authProviders, isNew, mcps } = data;
	let loginDialog = $state<HTMLDialogElement>();
	let overrideRedirect = $state<string | null>(null);
	let signUp = $state(true);

	onMount(() => {
		if (browser && new URL(window.location.href).searchParams.get('rd')) {
			loginDialog?.showModal();
		}
	});

	let sortedMcps = $derived(mcps.sort(sortByPreferredMcpOrder));
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

	$effect(() => {
		if (profile.current.loaded) {
			goto(`/catalog${isNew ? '?new' : ''}`, { replaceState: true });
		}
	});

	function closeLoginDialog() {
		loginDialog?.close();
		signUp = true;
	}

	const obotTiles = [
		{
			title: 'Add connectors to your system and APIs',
			description: 'Obot can connect to your existing systems and APIs to automate your workflows.',
			image: '/landing/images/obot-landing-connector.webp',
			tag: 'Performance'
		},
		{
			title: 'Automate Obots to create powerful AI agents',
			description: 'Obot can automate your Obots to create powerful AI agents.',
			image: '/landing/images/obot-landing-automation.webp',
			tag: 'Automation'
		},
		{
			title: 'Add data and information to Obots using RAG',
			description: 'Obot can add data and information to your Obots using RAG.',
			image: '/landing/images/obot-landing-rag.webp',
			tag: 'Data'
		},
		{
			title: 'Share Obots with anyone',
			description: 'Obots can be shared with anyone, and can be used by anyone.',
			image: '/landing/images/obot-landing-sharing.webp',
			tag: 'Collaboration'
		}
	];
</script>

{#snippet navLinks()}
	<a
		href="https://docs.obot.ai"
		class={responsive.isMobile ? 'icon-button' : 'nav-link'}
		rel="external"
		target="_blank">Docs</a
	>
	<a
		href="https://docs.obot.ai/blog"
		class={responsive.isMobile ? 'icon-button' : 'nav-link'}
		rel="external"
		target="_blank">Blog</a
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

<svelte:head>
	<title>Obot - Do more with AI</title>
</svelte:head>

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
			class="well flex w-full flex-col gap-4 overflow-hidden md:mt-8 md:max-w-(--breakpoint-2xl)"
		>
			{#if responsive.isMobile}
				<div class="mt-8 mb-6 w-full">
					{@render newsPill()}
				</div>
			{/if}
			<div class="flex w-full flex-col gap-4 md:flex-row">
				<div class="flex grow flex-col justify-center md:gap-16">
					{#if !responsive.isMobile}
						{@render newsPill()}
					{/if}

					<div class="flex flex-col">
						<h1 class="text-4xl font-bold md:text-5xl lg:text-6xl xl:text-7xl">
							Introducing Obot:
						</h1>
						<h1 class="text-4xl font-bold md:text-5xl lg:text-6xl xl:text-7xl">
							Build AI agents <br /> with MCP
						</h1>
					</div>
				</div>
				<div class="mt-4 flex flex-col gap-4 md:mt-0">
					{#if sortedMcps.length > 0}
						{@const firstFourMcps = sortedMcps.slice(0, 4)}
						<div
							class="relative my-4 grid w-full grid-cols-1 gap-y-2 md:w-sm md:gap-y-4 lg:w-lg xl:w-xl"
						>
							{#each firstFourMcps as mcp}
								<button
									onclick={() => {
										overrideRedirect = `/mcp?id=${mcp.id}`;
										loginDialog?.showModal();
									}}
									class="group from-surface2 to-surface1 relative flex cursor-pointer overflow-hidden rounded-2xl bg-gradient-to-r transition-all duration-300 hover:scale-105 md:rounded-l-4xl"
								>
									<div
										class="absolute -top-4 -right-5 z-20 h-[calc(100%+24px)] w-0 bg-gradient-to-l from-white/100 to-white/0 md:w-xs dark:from-black/100 dark:to-black/0"
									></div>
									<div class="relative z-20 flex w-full items-center gap-4 p-4">
										<img
											src={mcp.server.icon}
											alt={`${mcp.server.name} logo`}
											class="size-12 md:size-16 lg:size-24"
										/>
										<div class="flex w-full flex-col">
											<div
												class="flex h-full w-full flex-col gap-1 text-left text-black dark:text-white"
											>
												<h4 class="text-base font-semibold md:text-xl">{mcp.server.name}</h4>
												<p class="max-w-full text-xs font-light md:text-sm lg:max-w-xs xl:max-w-sm">
													{mcp.server.description}
												</p>
												<div
													class="border-surface3 group-hover:bg-surface3 flex w-fit items-center gap-1 rounded-xl border px-4 py-1 text-sm text-gray-500 transition-colors duration-300 group-hover:text-inherit"
												>
													Launch <ChevronsRight class="size-4" />
												</div>
											</div>
										</div>
									</div>
								</button>
							{/each}
							{#if !responsive.isMobile}
								<div class="flex items-center">
									<p class="text-md px-6 font-medium">Or if you’re feeling adventurous</p>
									<button
										onclick={() => {
											if (browser) {
												overrideRedirect = null;
												loginDialog?.showModal();
											}
										}}
										class="group from-surface2 to-surface1 group relative flex grow cursor-pointer overflow-hidden rounded-2xl bg-gradient-to-r transition-all duration-300 hover:scale-105 md:rounded-l-4xl"
									>
										<div
											class="absolute -top-4 -right-5 z-20 h-[calc(100%+24px)] w-0 bg-gradient-to-l from-white/100 to-white/0 md:w-[200px] dark:from-black/100 dark:to-black/0"
										></div>
										<div class="relative z-20 flex w-full items-center gap-4 p-4">
											<div class="flex w-fit items-center gap-1 px-4 py-1 text-sm">
												See More <ChevronsRight class="size-4" />
											</div>
										</div>
									</button>
								</div>
							{/if}
						</div>
					{/if}
				</div>
			</div>
		</div>
		<div
			class="well flex w-full max-w-(--breakpoint-2xl) flex-col items-center gap-6 overflow-hidden md:gap-12"
		>
			<div class="flex max-w-4xl grow flex-col items-center">
				<span class="text-blue text-md font-medium md:text-lg">Getting Started</span>
				<h2 class="mb-2 text-2xl font-semibold md:text-4xl">Launch Your First Obot</h2>
				<p
					class="text-md text-center font-light text-gray-500 md:mt-4 md:text-lg dark:text-gray-300"
				>
					Obots can work with a wide variety of tools to accomplish amazing things. A great way to
					get started is with an Obot that works with a single bundle. Try any of these, or if
					you’re feeling adventurous, we have a large variety of other tools to choose from.
				</p>
			</div>
			<div class="grid w-full grid-cols-1 gap-8 pb-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4">
				{#each sortedMcps.slice(4, 12) as mcp}
					<FeaturedMcpCard
						{mcp}
						onSubmit={() => {
							overrideRedirect = `/mcp?id=${mcp.id}`;
							loginDialog?.showModal();
						}}
					/>
				{/each}
			</div>
			<div class="flex w-full justify-center">
				<button
					class="button-primary"
					onclick={() => {
						overrideRedirect = null;
						loginDialog?.showModal();
					}}>See the Obot Catalog</button
				>
			</div>
		</div>
		<div class="well flex w-full max-w-(--breakpoint-xl) flex-col overflow-hidden">
			<div class="flex gap-4">
				<img src="/user/images/obot-icon-blue.svg" alt="obot logo" class="size-10 md:size-18" />
				<div class="flex flex-col">
					<span class="text-blue text-md gap-2 font-medium md:text-lg">
						Build your first Obot in X minutes!
					</span>
					<h2 class="w-full text-2xl font-semibold md:text-4xl">Everything You Need to Know</h2>
				</div>
			</div>
			<div class="border-surface2 mt-8 w-full rounded-2xl border-2 p-2">
				<div class="border-surface3 w-full overflow-hidden rounded-2xl border-2 bg-black shadow-md">
					<video controls title="nyan cat" class="aspect-video min-w-full">
						<track kind="captions" srclang="en" label="English" />
						<source src="" type="video/mp4" />
					</video>
				</div>
			</div>
		</div>
		<div class="well flex w-full max-w-(--breakpoint-xl) flex-col gap-6 overflow-hidden md:gap-12">
			<div class="flex w-full flex-col items-center">
				<span class="text-blue text-md font-medium md:text-lg">Extend Obot</span>
				<h2 class="text-2xl font-semibold md:text-4xl">Create Agents That Meet Your Needs</h2>
			</div>
			<div class="grid grid-cols-1 gap-8 md:grid-cols-2">
				{#each obotTiles as obotTile}
					<div class="bg-surface1 overflow-hidden rounded-2xl shadow-md">
						<img
							src={obotTile.image}
							class="h-[250px] w-full object-cover"
							alt="obot needs option-a"
						/>
						<div class="flex flex-col p-4">
							<span class="text-blue text-sm font-medium">{obotTile.tag}</span>
							<h4 class="mb-2 text-base leading-5 font-semibold md:text-lg">
								{obotTile.title}
							</h4>
							<p
								class="md:text-md mb-2 text-sm leading-4.5 font-light text-gray-500 dark:text-gray-300"
							>
								{obotTile.description}
							</p>
						</div>
					</div>
				{/each}
			</div>
		</div>
		<div class="well mb-16 flex max-w-(--breakpoint-2xl) gap-6 overflow-hidden md:gap-12">
			<div class="md:7/12 flex flex-col gap-8 lg:flex-5/12">
				<div class="flex flex-col">
					<span class="text-blue text-md font-medium md:text-lg">Learn More</span>
					<h2 class="text-2xl leading-7 font-semibold md:text-4xl md:leading-10">
						An Open Source Platform <br /> for AI Agents
					</h2>
				</div>
				<p class="text-md font-light text-gray-500 md:text-lg dark:text-gray-300">
					Obot is software you can run yourself. With Obot, any organization can deliver AI Agents
					as a service to employees.
				</p>

				<div class="flex flex-col items-center justify-center gap-8">
					<div class="flex gap-2">
						<img
							alt="github logo"
							src="/user/images/github-mark/github-mark.svg"
							class="mt-1 size-5 flex-shrink-0"
						/>
						<div class="flex w-full flex-col gap-4">
							<p class="text-base">
								<b>We're on Github!</b> We believe in building in the open—explore our source code, contribute
								to the project, or star us on GitHub to join a growing community of developers shaping
								the future of AI agents together.
							</p>
							<a
								href="https://github.com/obot-platform/obot"
								class="button-primary w-xs max-w-full text-center"
							>
								Download Obot
							</a>
						</div>
					</div>
					<div class="flex gap-2">
						<CalendarDays class="mt-1 size-5 flex-shrink-0" />
						<div class="flex w-full flex-col gap-4">
							<p class="text-base">
								<b>Want us to show you around?</b> Discover how effortlessly you can design, deploy,
								and scale powerful AI agents tailored to your workflow—
								<b><i>schedule a free personalized demo</i></b>
								today and let us show you exactly how our platform can supercharge your productivity,
								automate the mundane, and unlock new levels of efficiency for your team.
							</p>
							<button class="button-secondary w-xs max-w-full">Schedule a demo</button>
						</div>
					</div>
				</div>
			</div>
			{#if !responsive.isMobile}
				<div class="md:5/12 flex overflow-hidden rounded-l-2xl lg:flex-7/12">
					<img
						src="/landing/images/obot-landing-2.png"
						class="h-full object-cover"
						alt="obot landing-page obot-2"
					/>
				</div>
			{/if}
		</div>
	</main>
	<Footer />

	<!-- Login Modal -->
	<dialog
		bind:this={loginDialog}
		use:clickOutside={closeLoginDialog}
		class="fixed top-1/2 left-1/2 m-0 h-dvh max-h-none w-full max-w-none -translate-x-1/2 -translate-y-1/2 rounded-none p-4 shadow-lg backdrop:bg-black/50 md:max-h-fit md:max-w-md md:rounded-3xl"
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
					interacting with your own Obot.
				{:else}
					Welcome back! Log back in to start creating or interacting with your Obot again.
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
					href="/oauth2/start?rd={overrideRedirect !== null
						? overrideRedirect
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

{#snippet newsPill()}
	<div
		class="border-surface2 flex w-fit items-center gap-4 rounded-full border px-4 py-2 text-xs font-light text-gray-500 md:text-sm dark:text-gray-300"
	>
		<span class="flex grow truncate"
			><b class="font-semibold">News</b>: Obot 0.8.2 released today...</span
		>
		<a
			href="https://blog.obot.ai"
			class="text-blue flex flex-shrink-0 items-center gap-1 font-semibold"
		>
			Read More <ChevronsRight class="size-3" />
		</a>
	</div>
{/snippet}

<style lang="postcss">
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

	.nav-link {
		position: relative;
		height: 100%;
		display: inline-block;
		padding: 1.25rem 0.5rem;
		&::after {
			content: '';
			position: absolute;
			bottom: 0;
			left: 0;
			width: 100%;
			height: 4px;
			background-color: var(--color-blue);
			opacity: 0;
			transition: opacity 0.3s ease;
		}
		&:hover::after {
			opacity: 1;
		}
	}
</style>
