<script lang="ts">
	import { browser } from '$app/environment';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import type { Model } from '$lib/services/index.js';
	import { darkMode } from '$lib/stores/index.js';
	import { Box, ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { fade } from 'svelte/transition';

	let { data } = $props();
	let { models } = data;

	let search = $state('');
	let page = $state(0);
	let pageSize = $state(50);
	let filteredData = $derived(
		search
			? models.filter((model) => {
					const searchLower = search.toLowerCase();
					return (
						model.name.toLowerCase().includes(searchLower) ||
						model.modelProviderName.toLowerCase().includes(searchLower)
					);
				})
			: models
	);
	let paginatedData = $derived(filteredData.slice(page * pageSize, (page + 1) * pageSize));
	let selectedModel = $state<Model>();

	let connectDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	function convertToReadableUsage(usage: string) {
		if (usage === 'llm') return 'LLM';
		if (usage === 'image-generation') return 'Image Generation';
		if (usage === 'text-embedding') return 'Text Embedding';
		if (usage === 'vision') return 'Vision';
		if (usage === 'other') return 'Other';
		if (usage === '') return 'Unknown';
		return usage;
	}
</script>

<Layout showUserLinks>
	<div class="flex flex-col gap-8 pt-4" in:fade>
		<h1 class="text-2xl font-semibold">Models</h1>
		<div class="flex flex-col gap-4">
			<Search
				class="dark:bg-surface1 dark:border-surface3 bg-white shadow-sm dark:border"
				onChange={(val) => {
					search = val;
					page = 0;
				}}
				placeholder="Search by name..."
			/>
			<div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
				{#each paginatedData as item (item.id)}
					{@render modelCard(item)}
				{/each}
			</div>
			{#if filteredData.length > pageSize}
				<div
					class="bg-surface1 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 items-center justify-center gap-4 p-2 md:w-[calc(100%+4em)] md:-translate-x-8 dark:bg-black"
				>
					<button
						class="button-text flex items-center gap-1 disabled:no-underline disabled:opacity-50"
						onclick={() => (page = page - 1)}
						disabled={page === 0}
					>
						<ChevronLeft class="size-4" /> Previous
					</button>
					<span class="text-sm text-gray-400 dark:text-gray-600">
						{page + 1} of {Math.ceil(filteredData.length / pageSize)}
					</span>
					<button
						class="button-text flex items-center gap-1 disabled:no-underline disabled:opacity-50"
						onclick={() => (page = page + 1)}
						disabled={page === Math.floor(filteredData.length / pageSize)}
					>
						Next <ChevronRight class="size-4" />
					</button>
				</div>
			{:else}
				<div class="min-h-8 w-full"></div>
			{/if}
		</div>
	</div>
</Layout>

{#snippet modelCard(model: Model)}
	<div class="relative flex gap-2">
		<button
			class="dark:bg-surface1 dark:border-surface3 flex h-full w-full items-center gap-2 rounded-sm border border-transparent bg-white p-3 text-left shadow-sm"
			onclick={() => {
				selectedModel = model;
				connectDialog?.open();
			}}
		>
			<div
				class="flex-shrink-0 rounded-md bg-transparent p-0.5"
				class:dark:bg-gray-600={darkMode.isDark && !model.iconDark}
			>
				{#if darkMode.isDark && model.iconDark}
					<img src={model.iconDark} alt={model.name} class="size-8" />
				{:else if model.icon}
					<img src={model.icon} alt={model.name} class="size-8" />
				{:else}
					<Box class="size-8" />
				{/if}
			</div>
			<div class="flex flex-col">
				<div class="flex items-center gap-2 pr-6">
					<div class="line-clamp-1 text-xs font-light">{model.modelProviderName}</div>
					{#if model.usage}
						<div
							class="border-surface3 w-fit rounded-lg border px-2 text-[10px] font-light text-gray-400 dark:text-gray-600"
						>
							{convertToReadableUsage(model.usage)}
						</div>
					{/if}
				</div>
				<div class="mt-1 line-clamp-1 text-sm font-semibold">{model.name}</div>
			</div>
		</button>
		<div
			class="absolute -top-2 right-0 flex h-full translate-y-2 flex-col justify-between gap-4 p-2"
		>
			<DotDotDot
				class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
			>
				<div class="default-dialog flex min-w-max flex-col p-2">
					<button
						class="menu-button"
						onclick={() => {
							selectedModel = model;
							connectDialog?.open();
						}}
					>
						Get Connection URL
					</button>
				</div>
			</DotDotDot>
		</div>
	</div>
{/snippet}

<ResponsiveDialog bind:this={connectDialog} animate="slide">
	{#snippet titleContent()}
		<div
			class="bg-surface1 rounded-sm p-1 dark:bg-gray-600"
			class:dark:bg-gray-600={darkMode.isDark && !selectedModel?.iconDark}
		>
			{#if darkMode.isDark && selectedModel?.iconDark}
				<img src={selectedModel.iconDark} alt={selectedModel.name} class="size-8" />
			{:else if selectedModel?.icon}
				<img src={selectedModel.icon} alt={selectedModel.name} class="size-8" />
			{:else}
				<Box class="size-8" />
			{/if}
		</div>
		{selectedModel?.name}
	{/snippet}

	{#if browser}
		<div class="mb-8 flex flex-col gap-1">
			<label for="connectURL" class="font-light">Connection URL</label>
			<div class="mock-input-btn flex w-full items-center justify-between gap-2 shadow-inner">
				<p>
					{window.location.origin}/m/{selectedModel?.id}
				</p>
				<CopyButton
					showTextLeft
					text="{window.location.origin}/m/{selectedModel?.id}"
					classes={{
						button: 'flex-shrink-0 flex items-center gap-1 text-xs font-light hover:text-blue-500'
					}}
				/>
			</div>
		</div>
	{/if}
</ResponsiveDialog>

<svelte:head>
	<title>Obot | Models</title>
</svelte:head>
