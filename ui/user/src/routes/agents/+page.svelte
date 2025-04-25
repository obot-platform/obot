<script lang="ts">
	import { EditorService, type ProjectShare } from '$lib/services';
	import Notifications from '$lib/components/Notifications.svelte';
	import type { PageProps } from './$types';
	import ObotCard from '$lib/components/ObotCard.svelte';
	import { q, qIsSet } from '$lib/url';
	import { ChevronLeft, Plus } from 'lucide-svelte';
	import FeaturedAgentCard from '$lib/components/FeaturedAgentCard.svelte';
	import { sortByFeaturedNameOrder } from '$lib/sort';
	import { goto } from '$app/navigation';
	import Navbar from '$lib/components/Navbar.svelte';
	import { errors } from '$lib/stores';

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
	<Navbar />

	<main
		class="colors-background relative flex w-full flex-col items-center justify-center gap-12 pb-12"
	>
		{#if qIsSet('from')}
			{@const from = decodeURIComponent(q('from'))}
			<div class="mt-8 flex w-full flex-col justify-start md:px-8">
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
			class:mt-12={!qIsSet('from')}
		>
			{#if qIsSet('new')}
				<h2 class="text-3xl font-semibold md:text-4xl">Welcome To Obot</h2>
			{:else}
				<h2 class="text-3xl font-semibold md:text-4xl">Agent Catalog</h2>
			{/if}
			<p class="mb-4 max-w-full text-center text-base font-light md:max-w-md">
				Check out our featured agents below, or browse all agents to find the perfect one for you.
				Or if you're feeling adventurous, get started and create your own agent!
			</p>

			<div class="mb-8 flex w-full items-center justify-center px-4">
				<button
					class="bg-surface1 hover:bg-surface2 w-full rounded-xl p-4 shadow-md transition-colors duration-300 md:w-lg"
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
						<div class="flex -translate-y-1 flex-col gap-1 text-left">
							<h4 class="text-lg font-semibold">Create New Agent</h4>
							<p class="text-gray text-sm leading-4 font-light">
								Create an agent that fits your needs!
							</p>
						</div>
					</div>
				</button>
			</div>
		</div>
		{#if featured.length > 0}
			<div class="mb-4 flex w-full flex-col items-center justify-center">
				<div class="flex w-full max-w-(--breakpoint-xl) flex-col gap-4 px-4 md:px-12">
					<h3 class="text-2xl font-semibold md:text-3xl">Featured</h3>
					<div class="grid grid-cols-1 gap-x-4 gap-y-6 sm:gap-y-8 lg:grid-cols-2">
						{#each featured.slice(0, 4) as featuredShare}
							<FeaturedAgentCard project={featuredShare} {tools} />
						{/each}
					</div>
				</div>
			</div>
		{/if}

		<div class="flex w-full max-w-(--breakpoint-xl) flex-col">
			<div
				class="sticky top-0 z-30 flex items-center gap-4 bg-white px-4 pt-4 pb-2 md:px-12 dark:bg-black"
			>
				<h3 class="text-2xl font-semibold">More Agents</h3>
			</div>
			<div class="grid grid-cols-1 gap-4 px-4 pt-2 md:grid-cols-2 md:px-12 lg:grid-cols-3">
				{#each featured.slice(4) as project}
					<ObotCard {project} {tools} />
				{/each}
			</div>
		</div>
	</main>

	<Notifications />
</div>

<svelte:head>
	<title>Obot | Agents</title>
</svelte:head>
