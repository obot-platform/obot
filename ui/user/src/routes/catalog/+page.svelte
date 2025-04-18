<script lang="ts">
	import { darkMode, errors } from '$lib/stores';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { EditorService, type ProjectShare } from '$lib/services';
	import { responsive } from '$lib/stores';
	import Notifications from '$lib/components/Notifications.svelte';
	import type { PageProps } from './$types';
	import ObotCard from '$lib/components/ObotCard.svelte';
	import { q, qIsSet } from '$lib/url';
	import { ChevronLeft, Plus } from 'lucide-svelte';
	import FeaturedObotCard from '$lib/components/FeaturedObotCard.svelte';
	import { sortByFeaturedNameOrder } from '$lib/sort';
	import { goto } from '$app/navigation';

	let { data }: PageProps = $props();
	let featured = $state<ProjectShare[]>(
		data.shares.filter((s) => s.featured).sort(sortByFeaturedNameOrder)
	);
	let tools = $state(new Map(data.tools.map((t) => [t.id, t])));

	async function createNew() {
		try {
			const project = await EditorService.createObot();
			await goto(`/o/${project.id}`);
		} catch (error) {
			errors.append((error as Error).message);
		}
	}
</script>

<div class="flex h-full flex-col items-center">
	<div class="flex h-16 w-full items-center p-4 md:p-5">
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
					<ChevronLeft class="size-5" /> Back To Chat
				</a>
			</div>
		{/if}
		<div
			class="flex w-full max-w-(--breakpoint-xl) flex-col items-center justify-center gap-2 px-4 py-4"
		>
			{#if qIsSet('new')}
				<h2 class="text-3xl font-semibold md:text-4xl">Welcome To Obot</h2>
			{:else}
				<h2 class="text-3xl font-semibold md:text-4xl">Obot Catalog</h2>
			{/if}
			<p class="mb-4 max-w-full text-center text-base font-light md:max-w-md">
				Check out our featured obots below, or browse all obots to find the perfect one for you. Or
				if you're feeling adventurous, get started and create your own obot!
			</p>
		</div>
		{#if featured.length > 0}
			<div class="mb-4 flex w-full flex-col items-center justify-center">
				<div class="flex w-full max-w-(--breakpoint-xl) flex-col gap-4 px-4 md:px-12">
					<h3 class="text-2xl font-semibold md:text-3xl">Featured</h3>
					<div class="featured-card-layout gap-x-4 gap-y-6 sm:gap-y-8">
						{#each featured.slice(0, 4) as featuredShare}
							<FeaturedObotCard project={featuredShare} {tools} />
						{/each}
					</div>

					<p class="text-center text-lg font-light">Couldn't find what you were looking for?</p>

					<div class="mb-8 flex w-full items-center justify-center">
						<button
							class="bg-surface1 hover:bg-surface2 rounded-xl p-4 shadow-md transition-all duration-300"
							onclick={createNew}
						>
							<div class="flex w-full items-center gap-3">
								<div class="relative flex-shrink-0">
									<Plus
										class="absolute top-1/2 left-1/2 z-10 size-12 -translate-x-1/2 -translate-y-1/2 text-white opacity-90 dark:opacity-75"
									/>
									<img
										alt="obot create placeholder logo"
										src="/agent/images/obot_placeholder.webp"
										class="flex size-16 flex-shrink-0 rounded-full opacity-65 shadow-md shadow-gray-500 dark:shadow-black"
									/>
								</div>
								<div class="flex flex-col gap-1 text-left">
									<h4 class="text-lg font-semibold">Create New Obot</h4>
									<p class="text-gray text-sm leading-4 font-light">
										Make an Obot that fits your needs!
									</p>
								</div>
							</div>
						</button>
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
