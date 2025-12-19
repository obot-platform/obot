<script lang="ts">
	import {
		Plus,
		Server,
		Trash2,
		ChevronDown,
		ChevronUp,
		LoaderCircle,
		AlertTriangle,
		RefreshCcw
	} from 'lucide-svelte';
	import { onMount } from 'svelte';
	import {
		AdminService,
		type CompositeCatalogConfig,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type CompositeServerToolRow
	} from '$lib/services';
	import CompositeToolsSetup from './composite/CompositeSelectServerAndToolsSetup.svelte';
	import { slide } from 'svelte/transition';
	import { SvelteMap } from 'svelte/reactivity';
	import Toggle from '../Toggle.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		id?: string; // Composite catalog entry ID (when editing an existing composite)
		config: CompositeCatalogConfig;
		readonly?: boolean;
		catalogId?: string;
	}

	let { config = $bindable(), readonly, catalogId, id }: Props = $props();
	let componentEntries = $state<MCPCatalogEntry[]>([]);
	const componentServers = new SvelteMap<string, MCPCatalogServer>();
	let expanded = $state<Record<string, boolean>>({});
	let expandedTools = $state<Record<string, boolean>>({});
	let loading = $state(false);

	let configuringEntry = $state<MCPCatalogEntry | MCPCatalogServer>();
	let toolsByEntry = $state<Record<string, CompositeServerToolRow[]>>({});
	let toolsToEdit = $state<CompositeServerToolRow[]>([]);
	let populatedByEntry = $state<Record<string, boolean>>({});
	let loadingByEntry = $state<Record<string, boolean>>({});
	let configuringComponentId = $state<string | undefined>();
	let configuringIsNewComponent = $state<boolean>(false);
	// Track the initial component IDs that were loaded from the API (persisted components)
	let initialComponentIds = $state<Set<string>>(new Set());

	let compositeToolsSetupDialog = $state<ReturnType<typeof CompositeToolsSetup>>();

	const excluded = $derived([
		...(config?.componentServers ?? []).map((c) => getComponentId(c)),
		...(id ? [id] : [])
	]);

	// Helper to get unique ID for a component (catalogEntryID or mcpServerID)
	function getComponentId(c: { catalogEntryID?: string; mcpServerID?: string }): string {
		return c.catalogEntryID || c.mcpServerID || '';
	}

	// Build a configuring entry backed by the composite's manifest snapshot when
	// configuring tools for an existing entry
	function buildCompositeConfiguringEntry(
		componentId: string
	): MCPCatalogEntry | MCPCatalogServer | undefined {
		const component = config.componentServers?.find((c) => getComponentId(c) === componentId);
		if (!component || !component.manifest || (!component.catalogEntryID && !component.mcpServerID))
			return undefined;

		if (component.mcpServerID) {
			// This is a multi-user server, we should always use the live value since they should always exist
			const catalogServer = componentServers.get(component.mcpServerID);
			if (catalogServer) return catalogServer;

			throw new Error(`Catalog server not found for ID: ${component.mcpServerID}`);
		}

		const catalogEntry = componentEntries.find((e) => e.id === componentId);
		if (catalogEntry) {
			return {
				...catalogEntry,
				manifest: component.manifest
			};
		}

		// Fallback minimal entry if metadata isn't loaded; sufficient for Configure Tools.
		return {
			id: componentId,
			created: new Date().toISOString(),
			manifest: component.manifest,
			sourceURL: undefined,
			userCount: undefined,
			type: 'catalog-entry',
			isCatalogEntry: !component.mcpServerID,
			needsUpdate: false
		};
	}

	// Check if a component is newly added (not yet persisted to the composite entry)
	function isComponentNew(componentId: string): boolean {
		// A component is new if it wasn't part of the initial loaded state
		return !initialComponentIds.has(componentId);
	}

	// Open the tools configuration / refresh flow for a specific component
	function openToolsConfigurator(componentId: string) {
		const match =
			componentServers.get(componentId) || componentEntries.find((e) => e.id === componentId);
		if (match) {
			configuringEntry = match;
		}
		configuringComponentId = componentId;
		configuringIsNewComponent = false; // Refresh is always for existing components
		loadingByEntry[componentId] = true;
		toolsToEdit = toolsByEntry[componentId] ?? [];
		compositeToolsSetupDialog?.open();
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
				// Prefer the stored description snapshot when present; otherwise fall back to preview.
				const baseDescription = o.description ?? t?.description;

				// Pre-fill the editing fields with the effective values:
				// - name: overrideName if set, otherwise original name
				// - description: overrideDescription if set, otherwise base description
				const effectiveName = (o.overrideName || '').trim() || o.name;
				const effectiveDescription = (o.overrideDescription || '').trim() || baseDescription;

				return {
					id: `${componentId}-${effectiveName}`,
					name: o.name,
					overrideName: effectiveName,
					description: baseDescription,
					overrideDescription: effectiveDescription,
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
		// Capture the initial component IDs on first mount before any user interactions
		initialComponentIds = new Set((config?.componentServers || []).map((c) => getComponentId(c)));
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
	class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
>
	<h4 class="text-md font-semibold">Component Servers</h4>

	<div class="flex flex-col gap-2">
		{#if loading}
			<div class="text-on-surface1 text-sm">Loading component servers...</div>
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
							<Server class="text-on-surface1 size-8" />
						{/if}
						<div class="flex-1">
							<div class="font-medium">{entry.manifest?.name || 'Unnamed Server'}</div>
						</div>
						{#if entry.toolOverrides?.length && !readonly}
							<button
								type="button"
								class="icon-button"
								use:tooltip={'Refresh tool overrides'}
								disabled={loadingByEntry[componentId]}
								onclick={() => openToolsConfigurator(componentId)}
							>
								<RefreshCcw class={`size-4 ${loadingByEntry[componentId] ? 'animate-spin' : ''}`} />
							</button>
						{/if}
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
									<p class="text-on-surface1 text-sm font-light">
										All tools are enabled by default.
									</p>
									<p class="text-on-surface1 mb-4 text-sm font-light">
										Click below to further modify tool availability or details.
									</p>
									<button
										type="button"
										class="button-primary text-sm"
										disabled={loadingByEntry[componentId]}
										onclick={async () => {
											if (readonly) return;

											const entry = buildCompositeConfiguringEntry(componentId);
											configuringEntry = entry;
											configuringComponentId = componentId;
											configuringIsNewComponent = isComponentNew(componentId);
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
										{@const currentName = (tool.overrideName || '').trim() || tool.name}
										{@const currentDescription =
											(tool.overrideDescription || '').trim() || tool.description}
										{@const isCustomized =
											currentName !== tool.name || currentDescription !== tool.description}

										<div
											class="dark:bg-surface2 dark:border-surface3 flex items-start gap-2 rounded border border-transparent bg-white p-2 shadow-sm"
										>
											<div class="flex min-w-0 grow flex-col gap-2">
												<div class="flex items-start justify-between gap-2">
													<div class="min-w-0">
														<div class="truncate text-sm font-medium" title={currentName}>
															{currentName}
														</div>
														{#if currentDescription}
															<p class="line-clamp-2 text-xs" title={currentDescription}>
																{currentDescription}
															</p>
														{/if}
													</div>
													<div class="flex flex-shrink-0 items-center gap-2">
														<Toggle
															checked={tool.enabled === true}
															onChange={(checked) => {
																tool.enabled = checked;
															}}
															label="Enabled"
															disablePortal
														/>
														<button
															type="button"
															class="button px-3 py-1 text-xs"
															onclick={() => {
																const toolKey = `${componentId}-${tool.name}`;
																// When expanding, initialize inputs with current effective values
																if (!expandedTools[toolKey]) {
																	tool.overrideName = (tool.overrideName || '').trim() || tool.name;
																	tool.overrideDescription =
																		(tool.overrideDescription || '').trim() ||
																		tool.description ||
																		'';
																}
																expandedTools[toolKey] = !expandedTools[toolKey];
															}}
														>
															{expandedTools[`${componentId}-${tool.name}`]
																? 'Hide details'
																: 'Customize'}
														</button>
													</div>
												</div>

												{#if isCustomized}
													<div class="mt-1 flex items-center gap-1 text-[11px] text-amber-600">
														<AlertTriangle class="size-3 flex-shrink-0" />
														<p>
															Modified: This tool has been customized. The description or name has
															been changed.
														</p>
													</div>
												{/if}

												{#if expandedTools[`${componentId}-${tool.name}`]}
													<div class="mt-2 flex flex-col gap-2">
														<div class="flex flex-col gap-1">
															<p class="text-xs text-gray-500">Tool name</p>
															<input
																class="text-input-filled flex-1 text-sm"
																bind:value={tool.overrideName}
															/>
														</div>

														<div class="flex flex-col gap-1">
															<p class="text-xs text-gray-500">Description</p>
															<textarea
																class="text-input-filled h-24 resize-none text-xs"
																bind:value={tool.overrideDescription}
																placeholder="Enter tool description..."
															></textarea>
														</div>

														<div class="mt-2 flex justify-end">
															<button
																type="button"
																class="button px-3 py-1 text-xs"
																onclick={() => {
																	tool.overrideName = tool.name;
																	tool.overrideDescription = tool.description;
																}}
															>
																Reset to default
															</button>
														</div>
													</div>
												{/if}
											</div>
										</div>
									{/each}
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		{:else}
			<div class="text-on-surface1 text-sm">
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
				configuringComponentId = undefined;
				configuringIsNewComponent = true; // Adding a brand new component
				toolsToEdit = [];
				compositeToolsSetupDialog?.open();
			}}
			class="dark:bg-surface2 dark:border-surface3 dark:hover:bg-surface3 bg-background flex items-center justify-center gap-2 rounded-lg border border-gray-200 p-2 text-sm font-medium hover:bg-gray-50"
		>
			<Plus class="size-4" />
			Add MCP Server
		</button>
	{/if}
</div>

<CompositeToolsSetup
	bind:this={compositeToolsSetupDialog}
	{catalogId}
	{configuringEntry}
	compositeEntryId={id}
	componentId={configuringComponentId}
	isNewComponent={configuringIsNewComponent}
	existingTools={toolsToEdit}
	onCancel={() => {
		if (configuringEntry) {
			loadingByEntry[configuringEntry.id] = false;
		}
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

		loadingByEntry[id] = false;

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
