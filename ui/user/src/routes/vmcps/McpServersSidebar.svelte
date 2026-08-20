<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import McpDeprecatedNotice from '$lib/components/mcp/McpDeprecatedNotice.svelte';
	import type { MCPCatalogEntry } from '$lib/services';
	import { isDeprecatedMCPServer } from '$lib/services/user/mcp';
	import { mcpServersAndEntries, responsive } from '$lib/stores';
	import McpServerIcon from './McpServerIcon.svelte';
	import McpServersSettings from './McpServersSettings.svelte';
	import type { EntryDrag } from './entryDrag.svelte';
	import {
		buildMcpServerFilterOptions,
		filterMcpServersByCategories,
		isWorkspaceOwned,
		matchesQuery,
		MCP_SERVER_SORT_OPTIONS,
		sortMcpServers,
		type McpServerSortBy
	} from './utils';
	import { ChevronsRight, GripVertical, Plus, TriangleAlert } from '@lucide/svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		/** Exposed so the page can measure the panel and inset dialogs around it. */
		panelEl?: HTMLElement;
		open?: boolean;
		drag: EntryDrag;
		query?: string;
		onSearch: (value: string) => void;
		showAllConnectors?: boolean;
		canCreateEntry?: boolean;
	}

	let {
		panelEl = $bindable(),
		open = $bindable(true),
		drag,
		query = '',
		onSearch,
		showAllConnectors = false,
		canCreateEntry = false
	}: Props = $props();

	let sortBy = $state<McpServerSortBy>('popularity');
	let settings = $state({
		showDeprecatedServers: false,
		filterBy: ''
	});

	let eligibleEntries = $derived(
		mcpServersAndEntries.current.entries.filter(
			(entry) =>
				entry.manifest.runtime !== 'composite' &&
				entry.manifest.serverUserType !== 'multiUser' &&
				(settings.showDeprecatedServers || !isDeprecatedMCPServer(entry)) &&
				(showAllConnectors || !isWorkspaceOwned(entry))
		)
	);
	let filterOptions = $derived(buildMcpServerFilterOptions(eligibleEntries));
	let draggableEntries = $derived(
		sortMcpServers(
			filterMcpServersByCategories(eligibleEntries, settings.filterBy).filter((entry) =>
				query ? matchesQuery(entry, query) : true
			),
			sortBy
		)
	);
</script>

<div
	bind:this={panelEl}
	class={twMerge(
		'bg-base-100 dark:bg-base-300 border-base-300 overflow-y-auto border-l flex',
		responsive.isMobile
			? 'fixed z-40 h-[calc(100dvh-4rem)] w-dvw top-16 right-0'
			: 'static max-h-dvh',
		open ? (responsive.isMobile ? 'w-dvw' : 'w-4xl') : 'w-10'
	)}
>
	<button
		class="group h-full w-8 flex flex-col items-center justify-center gap-16 hover:bg-base-200 dark:hover:bg-base-400/50 transition-colors"
		onclick={() => (open = !open)}
		aria-label={open ? 'Hide MCP Servers' : 'Show MCP Servers'}
	>
		<ChevronsRight
			class={twMerge(
				'size-3 rotate-0 transition-all opacity-0 group-hover:opacity-100',
				!open && 'rotate-180'
			)}
		/>
		<p class="rotate-90 text-xs font-mono shrink-0 text-nowrap">
			<span class="group-hover:hidden">MCP Servers</span>
			<span class="hidden group-hover:inline">{open ? 'Hide' : 'Show'} MCP Servers</span>
		</p>
		<ChevronsRight
			class={twMerge(
				'size-3 rotate-0 transition-all opacity-0 group-hover:opacity-100',
				!open && 'rotate-180'
			)}
		/>
	</button>
	<div class="h-full flex flex-col grow" in:fly={{ x: 100 }}>
		{#if open}
			{@render selectionScreen()}
		{/if}
	</div>
</div>

{#snippet selectionScreen()}
	<div
		id="mcp-server-selection-screen"
		class="flex w-full min-w-0 flex-col overflow-y-auto pl-2 pr-4 pb-4"
	>
		<div class="sticky z-10 top-0 left-0 w-full bg-base-100 dark:bg-base-300 px-0 py-4">
			<Search
				value={query}
				class="text-sm"
				placeholder="Search MCP servers..."
				onChange={onSearch}
			/>
			<div class="flex items-center gap-2 mt-2">
				<label
					id="mcp-server-sort-by-label"
					for="mcp-server-sort-by"
					class="flex grow items-center"
				>
					<span
						class="text-sm font-light h-10 shrink-0 px-4 border border-base-300 borded-r-none rounded-l-sm flex items-center justify-center"
					>
						Sort by
					</span>
					<Select
						id="mcp-server-sort-by"
						options={MCP_SERVER_SORT_OPTIONS}
						bind:selected={sortBy}
						placeholder="Sort by"
						ariaLabelledby="mcp-server-sort-by-label"
						class="text-sm bg-base-200 dark:bg-base-100 shadow-inner! rounded-l-none rounded-r-sm"
						classes={{ root: 'w-full', option: 'text-sm' }}
					/>
				</label>
				<McpServersSettings bind:settings {filterOptions} />
			</div>
		</div>
		{#if draggableEntries.length > 0}
			<div class="flex flex-col gap-1">
				{#if canCreateEntry}
					{@render createEntryButton()}
				{/if}
				{#each draggableEntries as entry (entry.id)}
					{@render serverCard(entry)}
				{/each}
			</div>
		{:else}
			<p class="text-muted-content text-xs italic">No MCP servers available.</p>
		{/if}
	</div>
{/snippet}

{#snippet createEntryButton()}
	<button
		id="mcp-create-catalog-entry-button"
		type="button"
		class={twMerge(
			'group bg-base-100 dark:bg-base-300 border-dashed flex touch-none cursor-grab items-center rounded-lg border border-base-300 dark:border-base-400 transition-[transform,box-shadow,opacity] duration-150 select-none',
			'hover:bg-base-300 dark:hover:bg-base-200 border-base-300 dark:border-base-400',
			drag.isDraggingNewEntry && 'cursor-grabbing opacity-30'
		)}
		aria-label="Create a new entry, or drag it onto a vMCP or Create vMCP"
		onpointerdown={(event) => drag.pointerDown(event)}
		onpointermove={drag.pointerMove}
		onpointerup={drag.pointerUp}
		onpointercancel={drag.cancel}
		onkeydown={(event) => {
			if (event.key !== 'Enter' && event.key !== ' ') return;
			event.preventDefault();
			drag.activate();
		}}
	>
		<div class="shrink-0 pl-3">
			<GripVertical class="size-3" />
		</div>
		<div class="flex gap-2 grow h-full px-3 py-2 items-center">
			<div class="bg-primary/10 text-primary rounded-md p-2 shrink-0">
				<Plus class="size-5" />
			</div>
			<p
				class="inline items-center gap-0.5 text-center text-xs font-medium text-muted-content group-hover:text-base-content"
			>
				Create New Entry
			</p>
		</div>
	</button>
{/snippet}

{#snippet serverCard(entry: MCPCatalogEntry)}
	{@const dragging = drag.isDragging(entry)}
	{@const previewTools = (entry.manifest.toolPreview ?? []).slice(0, 3)}
	{#snippet previewPopover()}
		<div class="flex flex-col gap-2 text-left p-1">
			<div class="flex items-center gap-1">
				{#if entry.manifest.icon}
					<img src={entry.manifest.icon} alt="" class="size-5 icon shrink-0" />
				{/if}
				<p class="text-sm font-semibold">
					{entry.manifest.name}
				</p>
				{#if isDeprecatedMCPServer(entry)}
					<McpDeprecatedNotice deprecated />
				{/if}
			</div>

			{#if previewTools.length > 0}
				<div class="divider my-0 text-xs">Example Tools</div>
				<ul class="flex flex-col gap-2">
					{#each previewTools as tool (tool.id || tool.name)}
						<li>
							<p class="text-xs font-semibold">{tool.name}</p>
							{#if tool.description}
								<p class="text-muted-content line-clamp-1 font-light text-xs">{tool.description}</p>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}

			<div class="divider my-0"></div>

			<p class="text-muted-content text-xs font-light text-center">
				Click to view further details.
			</p>
		</div>
	{/snippet}

	<button
		id={`mcp-server-card-${entry.id}`}
		type="button"
		class={twMerge(
			'bg-base-100 dark:bg-base-300 flex touch-none cursor-grab items-center rounded-lg border border-base-300 dark:border-base-400 transition-[transform,box-shadow,opacity] duration-150 select-none',
			'hover:bg-base-300 dark:hover:bg-base-200 border-base-300 dark:border-base-400',
			dragging && 'cursor-grabbing opacity-30'
		)}
		aria-label={`View ${entry.manifest.name ?? 'server'} details, or drag it onto a vMCP or Create vMCP`}
		use:tooltip={dragging
			? undefined
			: (entry.manifest.toolPreview?.length ?? 0) > 0
				? {
						snippet: previewPopover,
						placement: 'left',
						// The panel stays interactive while the details dialog is open, so its hover
						// preview has to clear the dialog too.
						classes: ['max-w-xs', 'z-above-dialog', 'text-left', 'tooltip-surface']
					}
				: undefined}
		onpointerdown={(event) => drag.pointerDown(event, entry)}
		onpointermove={drag.pointerMove}
		onpointerup={drag.pointerUp}
		onpointercancel={drag.cancel}
		onkeydown={(event) => {
			if (event.key !== 'Enter' && event.key !== ' ') return;
			event.preventDefault();
			drag.activate(entry);
		}}
	>
		<div class="shrink-0 pl-3">
			<GripVertical class="size-3" />
		</div>
		<div class="flex gap-2 grow h-full px-3 py-2 items-center relative">
			{#if isDeprecatedMCPServer(entry)}
				<div
					class="badge badge-xs absolute top-1 right-1 badge-warning badge-soft bg-warning/10 border-transparent rounded-sm p-1"
					aria-label="This MCP server is deprecated"
				>
					<TriangleAlert class="size-3" />
				</div>
			{/if}
			<McpServerIcon icon={entry.manifest.icon} />
			<div class="flex flex-col gap-0.5 text-left">
				<p class="line-clamp-1 text-xs font-medium">
					{entry.manifest.name}
					{#each entry.manifest.metadata?.categories?.split(',') as category (category)}
						<span
							class="badge badge-xs badge-outline border-base-300 bg-base-300/30 dark:bg-base-400/30 dark:border-base-400 rounded-sm p-1 font-light mx-0.5"
						>
							{category}
						</span>
					{/each}
				</p>
				<p class="text-muted-content text-xs font-light line-clamp-2 tracking-tight">
					{entry.manifest.shortDescription || entry.manifest.description || 'No description'}
				</p>
			</div>
		</div>
	</button>
{/snippet}
