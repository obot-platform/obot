<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { stripMarkdownToText } from '$lib/markdown';
	import type { MCPCatalogServer, MCPCatalogEntry } from '$lib/services';
	import { parseCategories } from '$lib/services/chat/mcp';
	import { Server, Unplug } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		data:
			| MCPCatalogServer
			| MCPCatalogEntry
			| (MCPCatalogServer & { categories: string[] })
			| (MCPCatalogEntry & { categories: string[] });
		onClick: () => void;
		action?: Snippet;
	}

	let { data, onClick, action }: Props = $props();
	let icon = $derived(
		'manifest' in data ? data.manifest.icon : (data.commandManifest?.icon ?? data.urlManifest?.icon)
	);
	let name = $derived(
		'manifest' in data ? data.manifest.name : (data.commandManifest?.name ?? data.urlManifest?.name)
	);
	let categories = $derived('categories' in data ? data.categories : parseCategories(data));
</script>

<div class="relative flex flex-col">
	<button
		class="dark:bg-surface1 dark:border-surface3 flex h-full w-full flex-col rounded-sm border border-transparent bg-white p-3 text-left shadow-sm"
		onclick={onClick}
	>
		<div class="flex items-center gap-2 pr-6">
			<div
				class="flex size-8 flex-shrink-0 items-center justify-center self-start rounded-md bg-transparent p-0.5 dark:bg-gray-600"
			>
				{#if icon}
					<img src={icon} alt={name} />
				{:else}
					<Server />
				{/if}
			</div>
			<div class="flex max-w-[calc(100%-2rem)] flex-col">
				<p class="text-sm font-semibold">{name}</p>
				<span
					class={twMerge(
						'text-xs leading-4.5 font-light text-gray-400 dark:text-gray-600',
						categories.length > 0 ? 'line-clamp-2' : 'line-clamp-3'
					)}
				>
					{#if 'manifest' in data}
						{stripMarkdownToText(data.manifest.description ?? '')}
					{:else}
						{stripMarkdownToText(
							data.commandManifest?.description ?? data.urlManifest?.description ?? ''
						)}
					{/if}
				</span>
			</div>
		</div>
		<div class="flex w-full flex-wrap gap-1 pt-2">
			{#each categories as category (category)}
				<div
					class="border-surface3 rounded-full border px-1.5 py-0.5 text-[10px] font-light text-gray-400 dark:text-gray-600"
				>
					{category}
				</div>
			{/each}
		</div>
	</button>
	<div class="absolute -top-2 right-0 flex h-full translate-y-2 flex-col justify-between gap-4 p-2">
		{#if action}
			{@render action()}
		{:else}
			<button
				class="icon-button hover:bg-surface1 dark:hover:bg-surface2 size-6 min-h-auto min-w-auto flex-shrink-0 p-1 hover:text-blue-500"
				use:tooltip={'Connect to server'}
				onclick={onClick}
			>
				<Unplug class="size-4" />
			</button>
		{/if}
	</div>
</div>
