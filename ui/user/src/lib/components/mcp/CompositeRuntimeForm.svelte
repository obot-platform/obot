<script lang="ts">
	import { Plus, Server, Trash2, ChevronDown, ChevronUp, LoaderCircle } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import {
		AdminService,
		type CompositeCatalogConfig,
		type CompositeRuntimeConfig,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type CompositeServerToolRow
	} from '$lib/services';
	import type { AdminMcpServerAndEntriesContext } from '$lib/context/admin/mcpServerAndEntries.svelte';
	import CompositeToolsSetup from './composite/CompositeSelectServerAndToolsSetup.svelte';
	import { slide } from 'svelte/transition';
	import CompositeEditTools from './composite/CompositeEditTools.svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import Toggle from '../Toggle.svelte';

	interface Props {
		id?: string;
		config: CompositeCatalogConfig | CompositeRuntimeConfig;
		readonly?: boolean;
		catalogId?: string;
		mcpEntriesContextFn?: () => AdminMcpServerAndEntriesContext;
	}

	let { config = $bindable(), readonly, catalogId, mcpEntriesContextFn, id }: Props = $props();
	let componentEntries = $state<MCPCatalogEntry[]>([]);
	const componentServers = new SvelteMap<string, MCPCatalogServer>();
	let expanded = $state<Record<string, boolean>>({});
	let loading = $state(false);

	let configuringEntry = $state<MCPCatalogEntry | MCPCatalogServer>();
	let toolsByEntry = $state<Record<string, CompositeServerToolRow[]>>({});
	let populatedByEntry = $state<Record<string, boolean>>({});
	let loadingByEntry = $state<Record<string, boolean>>({});
	let toolsToEdit = $state<CompositeServerToolRow[]>([]);

	let compositeToolsSetupDialog = $state<ReturnType<typeof CompositeToolsSetup>>();
	let editCurrentToolsDialog = $state<ReturnType<typeof CompositeEditTools>>();

	const excluded = $derived([
		...(config?.componentServers ?? []).map((c) => getComponentId(c)),
		...(id ? [id] : [])
	]);

	// Helper to get unique ID for a component (catalogEntryID or mcpServerID)
	function getComponentId(c: { catalogEntryID?: string; mcpServerID?: string }): string {
		return c.catalogEntryID || c.mcpServerID || '';
	}

	// Pre-populate toolsByEntry from existing toolOverrides in config
	function prePopulateExistingToolOverrides() {
		if (!config?.componentServers) return;

		// Build a quick lookup for loaded catalog entries by id (to use their previews if needed)
		const entryById = new Map(componentEntries.map((e) => [e.id, e]));

		for (const component of config.componentServers) {
			const overrides = component.toolOverrides || [];
			if (!overrides.length) continue;

			const componentId = getComponentId(component);
			const manifestPreview = component.manifest?.toolPreview || [];
			const entryPreview = entryById.get(componentId)?.manifest?.toolPreview || [];
			const preview = manifestPreview.length ? manifestPreview : entryPreview;

			// If overrides exist, only show those overrides (use preview to enrich descriptions when present)
			// Preview of all tools should only be used when user explicitly populates for the first time
			const previewMap = new Map((preview || []).map((t) => [t.name, t]));
			const rows: CompositeServerToolRow[] = overrides.map((o) => {
				const t = previewMap.get(o.name);
				return {
					id: `${componentId}-${o.overrideName || o.name}`,
					originalName: o.name,
					overrideName: o.overrideName || o.name,
					originalDescription: t?.description || '',
					overrideDescription: o.overrideDescription || t?.description || '',
					enabled: o.enabled === true
				};
			});

			if (rows.length) {
				toolsByEntry[componentId] = rows;
				populatedByEntry[componentId] = true;
			}
		}
	}

	// Load full catalog entry and multi-user server details via APIs (no context)
	async function loadComponentEntries() {
		if (!config?.componentServers) return;

		loading = true;
		try {
			const catalogComponents = config.componentServers.filter((c) => c.catalogEntryID);
			const multiUserComponents = config.componentServers.filter((c) => c.mcpServerID);

			if (catalogId) {
				// Fetch entries and servers, then filter by current component IDs
				const [entries, servers] = await Promise.all([
					AdminService.listMCPCatalogEntries(catalogId, { all: true }) as Promise<
						MCPCatalogEntry[]
					>,
					AdminService.listMCPCatalogServers(catalogId, { all: true }) as Promise<
						MCPCatalogServer[]
					>
				]);

				const entryIds = new Set(catalogComponents.map((c) => c.catalogEntryID!));
				componentEntries = entries.filter((e) => entryIds.has(e.id));

				componentServers.clear();
				for (const component of multiUserComponents) {
					const server = servers.find((s) => s.id === component.mcpServerID);
					if (server) {
						componentServers.set(component.mcpServerID!, server);
					}
				}
			} else {
				// Unsaved composite: no catalog to fetch from
				componentEntries = [];
				componentServers.clear();
			}

			// Pre-populate existing tool overrides after entries are loaded
			prePopulateExistingToolOverrides();
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		loadComponentEntries();
	});

	function removeServer(componentId: string) {
		config.componentServers = (config.componentServers || []).filter(
			(c) => getComponentId(c) !== componentId
		) as unknown as typeof config.componentServers;
		componentEntries = componentEntries.filter((e) => e.id !== componentId);
		delete toolsByEntry[componentId];
		delete populatedByEntry[componentId];
		delete loadingByEntry[componentId];
	}
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
>
	<h4 class="text-md font-semibold">Component Servers</h4>

	<div class="flex flex-col gap-2">
		{#if loading}
			<div class="text-sm text-gray-500">Loading component servers...</div>
		{:else if config.componentServers.length > 0}
			{#each config.componentServers as entry (getComponentId(entry))}
				{@const componentId = getComponentId(entry)}
				<div
					class="dark:bg-surface2 dark:border-surface3 rounded-lg border border-gray-200 bg-gray-50"
				>
					<div class="flex items-center gap-3 p-3">
						{#if entry.manifest?.icon}
							<img src={entry.manifest.icon} alt={entry.manifest.name} class="size-8" />
						{:else}
							<Server class="size-8 text-gray-400" />
						{/if}
						<div class="flex-1">
							<div class="font-medium">{entry.manifest?.name || 'Unnamed Server'}</div>
						</div>
						<button
							type="button"
							class="icon-button"
							onclick={() => (expanded[componentId] = !expanded[componentId])}
							aria-label={expanded[componentId] ? 'Collapse' : 'Expand'}
						>
							{#if expanded[componentId]}
								<ChevronUp class="size-4" />
							{:else}
								<ChevronDown class="size-4" />
							{/if}
						</button>
						{#if !readonly}
							<button class="icon-button text-red-500" onclick={() => removeServer(componentId)}>
								<Trash2 class="size-4" />
							</button>
						{/if}
					</div>
					{#if expanded[componentId]}
						<div class="border-t border-gray-200 p-3" in:slide={{ axis: 'y' }}>
							{#if !populatedByEntry[componentId]}
								<div class="flex flex-col items-center justify-center pb-2">
									<p class="text-sm font-light text-gray-500">All tools are enabled by default.</p>
									<p class="mb-4 text-sm font-light text-gray-500">
										Click below to further modify tool availability or details.
									</p>
									<button
										type="button"
										class="button-primary text-sm"
										disabled={loadingByEntry[componentId]}
										onclick={async () => {
											const match =
												componentServers.get(componentId) ||
												componentEntries.find((e) => e.id === componentId);
											if (match) {
												configuringEntry = match;
											}
											compositeToolsSetupDialog?.open();
										}}
									>
										{#if loadingByEntry[componentId]}
											<LoaderCircle class="size-4 animate-spin" />
										{:else}
											Configure Tools
										{/if}
									</button>
								</div>
							{/if}
							{#if entry.toolOverrides?.length}
								<div class="flex flex-col gap-2">
									{#each entry.toolOverrides as tool, index (index)}
										<div
											class="dark:bg-surface2 dark:border-surface3 flex gap-2 rounded border border-transparent bg-white p-2 shadow-sm"
										>
											<div class="flex grow flex-col gap-1">
												<input
													class="text-input-filled flex-1 text-sm"
													bind:value={tool.overrideName}
													placeholder={tool.overrideName || tool.name}
												/>

												<textarea
													class="text-input-filled mt-1 resize-none text-xs"
													bind:value={tool.overrideDescription}
													placeholder="Enter tool description..."
													rows="2"
												></textarea>
											</div>

											<Toggle
												checked={tool.enabled ?? false}
												onChange={(checked) => {
													tool.enabled = checked;
												}}
												label="Enable/Disable Tool"
											/>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		{:else}
			<div class="text-sm text-gray-500 dark:text-gray-400">
				Select one or more MCP servers to include in the composite server. Users will see this as a
				single server with aggregated tools and resources.
			</div>
		{/if}
	</div>

	{#if !readonly}
		<button
			type="button"
			onclick={() => {
				configuringEntry = undefined;
				compositeToolsSetupDialog?.open();
			}}
			class="dark:bg-surface2 dark:border-surface3 dark:hover:bg-surface3 flex items-center justify-center gap-2 rounded-lg border border-gray-200 bg-white p-2 text-sm font-medium hover:bg-gray-50"
		>
			<Plus class="size-4" />
			Add MCP Server
		</button>
	{/if}
</div>

<CompositeToolsSetup
	bind:this={compositeToolsSetupDialog}
	{mcpEntriesContextFn}
	{catalogId}
	{configuringEntry}
	onCancel={() => {
		configuringEntry = undefined;
	}}
	onSuccess={(componentConfig, entry, tools) => {
		const id = getComponentId(componentConfig);
		const idx = (config.componentServers || []).findIndex((c) => getComponentId(c) === id);

		if (idx >= 0) {
			const prev = config.componentServers[idx];
			config.componentServers = [
				...config.componentServers.slice(0, idx),
				{ ...prev, ...componentConfig },
				...config.componentServers.slice(idx + 1)
			] as unknown as typeof config.componentServers;
		} else {
			config.componentServers = [
				...config.componentServers,
				componentConfig
			] as unknown as typeof config.componentServers;
		}

		if (tools) {
			populatedByEntry[id] = true;
			toolsByEntry[id] = tools;
		}

		if ('isCatalogEntry' in entry) {
			if (!componentEntries.find((e) => e.id === entry.id)) {
				componentEntries = [...componentEntries, entry];
			}
		} else {
			componentServers.set(entry.id, entry);
		}
	}}
	{excluded}
/>

<CompositeEditTools
	bind:this={editCurrentToolsDialog}
	{configuringEntry}
	tools={toolsToEdit}
	onSuccess={() => {
		if (!configuringEntry) return;
		config.componentServers = config.componentServers.map((c) => {
			const id = getComponentId(c);
			if (c.mcpServerID === id || c.catalogEntryID === id) {
				return {
					...c,
					toolOverrides: toolsToEdit
				};
			}
			return c;
		}) as unknown as typeof config.componentServers;
		toolsByEntry[configuringEntry.id] = toolsToEdit;
	}}
/>
