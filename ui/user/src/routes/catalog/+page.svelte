<script lang="ts">
	import { darkMode } from '$lib/stores';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { type ProjectShare } from '$lib/services';
	import { responsive } from '$lib/stores';
	import Notifications from '$lib/components/Notifications.svelte';
	import type { PageProps } from './$types';
	import ObotCard from '$lib/components/ObotCard.svelte';
	import { q, qIsSet } from '$lib/url';
	import { ChevronLeft } from 'lucide-svelte';
	import FeaturedObotCard from '$lib/components/FeaturedObotCard.svelte';
	import { sortByFeaturedNameOrder } from '$lib/sort';

	let { data }: PageProps = $props();
	let featured = $state<ProjectShare[]>(
		data.shares.filter((s) => s.featured).sort(sortByFeaturedNameOrder)
	);
	let tools = $state(new Map(data.tools.map((t) => [t.id, t])));
</script>

<div class="flex h-full flex-col items-center">
	<div class="flex h-16 w-full items-center p-4 md:p-5">
		<a href="/home" class="relative flex items-end">
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
		</a>
		<div class="grow"></div>
		<div class="flex items-center gap-1">
			{#if !responsive.isMobile}
				<a href="https://docs.obot.ai" rel="external" target="_blank" class="icon-button">Docs</a>
				<a href="https://discord.gg/9sSf4UyAMC" rel="external" target="_blank" class="icon-button">
					{#if darkMode.isDark}
						<img src="/user/images/discord-mark/discord-mark-white.svg" alt="Discord" class="h-6" />
					{:else}
						<img src="/user/images/discord-mark/discord-mark.svg" alt="Discord" class="h-6" />
					{/if}
				</a>
				<a
					href="https://github.com/obot-platform/obot"
					rel="external"
					target="_blank"
					class="icon-button"
				>
					{#if darkMode.isDark}
						<img src="/user/images/github-mark/github-mark-white.svg" alt="GitHub" class="h-6" />
					{:else}
						<img src="/user/images/github-mark/github-mark.svg" alt="GitHub" class="h-6" />
					{/if}
				</a>
			{/if}
			<Profile />
		</div>
	</div>

	<main class="colors-background relative flex w-full flex-col items-center justify-center pb-12">
		{#if qIsSet('from')}
			{@const from = decodeURIComponent(q('from'))}
			<div class="mt-8 flex w-full max-w-(--breakpoint-xl) flex-col justify-start md:px-8">
				<a
					href={from}
					class="button-text flex w-fit items-center gap-1 pb-0 text-base font-semibold text-black md:text-lg dark:text-white"
				>
					<ChevronLeft class="size-5" />{from.includes('home') ? 'My Obots' : 'Go Back'}
				</a>
			</div>
		{/if}
		{#if qIsSet('new')}
			<div
				class="flex w-full max-w-(--breakpoint-xl) flex-col items-center justify-center gap-2 px-4 py-4"
			>
				<h2 class="text-3xl font-semibold md:text-4xl">Welcome To Obot</h2>
				<p class="text-md mb-4 max-w-full text-center md:max-w-md">
					Check out our featured obots below, or browse all obots to find the perfect one for you.
					Or if you're feeling adventurous, get started and create your own obot!
				</p>
			</div>
		{/if}
		{#if featured.length > 0}
			<div class="mb-4 flex w-full flex-col items-center justify-center">
				<div class="flex w-full max-w-(--breakpoint-xl) flex-col gap-4 px-4 md:px-12">
					<h3 class="mt-8 text-2xl font-semibold md:text-3xl">Featured</h3>
					<div class="featured-card-layout gap-x-4 gap-y-6 sm:gap-y-8">
						{#each featured.slice(0, 4) as featuredShare}
							<FeaturedObotCard project={featuredShare} {tools} />
						{/each}
					</div>
				</div>
			</div>
		{/if}

		<div class="flex w-full max-w-(--breakpoint-xl) flex-col">
			<div
				class="sticky top-0 z-30 flex items-center gap-4 bg-white px-4 pt-4 pb-2 md:px-12 dark:bg-black"
			>
				<h3 class="text-2xl font-semibold">More Obots</h3>
			</div>
			<div class="card-layout px-4 pt-2 md:px-12">
				{#each data.shares.slice(4) as project}
					<ObotCard {project} {tools} />
				{/each}
			</div>
		</div>
	</main>

	<Notifications />
</div>

<svelte:head>
	<title>Obot | Home</title>
</svelte:head>
