<script lang="ts">
	import { Plus, Server, Trash2, ChevronDown, ChevronUp, LoaderCircle } from 'lucide-svelte';
	import SearchMcpServers from '../admin/SearchMcpServers.svelte';
	import { onMount } from 'svelte';
	import {
		AdminService,
		ChatService,
		type CompositeCatalogConfig,
		type CompositeRuntimeConfig,
		type MCPCatalogEntry,
		type MCPCatalogServer
	} from '$lib/services';
	import type { AdminMcpServerAndEntriesContext } from '$lib/context/admin/mcpServerAndEntries.svelte';
	import CatalogConfigureForm, { type LaunchFormData } from './CatalogConfigureForm.svelte';
	import { hasEditableConfiguration, convertEnvHeadersToRecord } from '$lib/services/chat/mcp';

	interface Props {
		config: CompositeCatalogConfig | CompositeRuntimeConfig;
		readonly?: boolean;
		catalogId?: string;
		mcpEntriesContextFn?: () => AdminMcpServerAndEntriesContext;
	}

	let { config = $bindable(), readonly, catalogId, mcpEntriesContextFn }: Props = $props();
	let searchDialog = $state<ReturnType<typeof SearchMcpServers>>();
	let componentEntries = $state<MCPCatalogEntry[]>([]);
	let componentServers = $state<Map<string, MCPCatalogServer>>(new Map());
	let expanded = $state<Record<string, boolean>>({});
	let loading = $state(false);

	type ToolRow = {
		id: string;
		originalName: string;
		overrideName: string;
		originalDescription?: string;
		overrideDescription?: string;
		enabled: boolean;
	};
	let toolsByEntry = $state<Record<string, ToolRow[]>>({});
	let populatedByEntry = $state<Record<string, boolean>>({});
	let loadingByEntry = $state<Record<string, boolean>>({});

	// Helper to get unique ID for a component (catalogEntryID or mcpServerID)
	function getComponentId(c: { catalogEntryID?: string; mcpServerID?: string }): string {
		return c.catalogEntryID || c.mcpServerID || '';
	}

	function updateCompositeToolMappings() {
		if (!config) return;
		const componentServers = (config.componentServers || []).map((c) => {
			const componentId = getComponentId(c);
			const rows = toolsByEntry[componentId] || [];
			const toolOverrides = rows.map((row) => ({
				name: row.originalName,
				overrideName: row.overrideName,
				overrideDescription: row.overrideDescription,
				enabled: row.enabled
			}));
			return {
				catalogEntryID: c.catalogEntryID,
				mcpServerID: c.mcpServerID,
				manifest: c.manifest,
				toolOverrides,
				disabled: c.disabled
			};
		});
		config.componentServers = componentServers as unknown as typeof config.componentServers;
	}

	// Per-entry configuration dialog state
	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configureForm = $state<LaunchFormData>();
	let configuringEntry = $state<MCPCatalogEntry>();

	function initConfigureForm(entry: MCPCatalogEntry) {
		configureForm = {
			envs: entry.manifest?.env?.map((env) => ({ ...env, value: '' })),
			headers: entry.manifest?.remoteConfig?.headers?.map((h) => ({ ...h, value: '' })),
			...(entry.manifest?.remoteConfig?.hostname
				? { hostname: entry.manifest.remoteConfig.hostname, url: '' }
				: {})
		};
	}

	async function runPreview(
		entry: MCPCatalogEntry,
		body: { config?: Record<string, string>; url?: string }
	) {
		if (!catalogId) return;
		loadingByEntry[entry.id] = true;
		try {
			const resp = (await AdminService.generateMcpCatalogEntryToolPreviews(
				catalogId!,
				entry.id,
				body,
				{ dryRun: true }
			)) as unknown as MCPCatalogEntry;
			const preview = resp?.manifest?.toolPreview || [];
			toolsByEntry[entry.id] = preview.map((t) => ({
				id: `${entry.id}-${t.id || t.name}`,
				originalName: t.name,
				overrideName: t.name,
				originalDescription: t.description,
				overrideDescription: t.description,
				enabled: true
			}));
			populatedByEntry[entry.id] = true;
			updateCompositeToolMappings();
		} catch (err: unknown) {
			const msg = err instanceof Error ? err.message : String(err);
			if (msg.includes('OAuth')) {
				const oauthURL = await AdminService.getMcpCatalogToolPreviewsOauth(
					catalogId!,
					entry.id,
					body,
					{ dryRun: true }
				);
				if (oauthURL) window.open(oauthURL, '_blank');
			} else {
				throw err;
			}
		} finally {
			loadingByEntry[entry.id] = false;
		}
	}

	async function populateMultiUserServerTools(serverId: string) {
		loadingByEntry[serverId] = true;
		try {
			const tools = await ChatService.listMcpCatalogServerTools(serverId);
			toolsByEntry[serverId] = tools.map((t) => ({
				id: `${serverId}-${t.id || t.name}`,
				originalName: t.name,
				overrideName: t.name,
				originalDescription: t.description,
				overrideDescription: t.description,
				enabled: t.enabled !== false
			}));
			populatedByEntry[serverId] = true;
			updateCompositeToolMappings();
		} catch (err: unknown) {
			console.error(`Failed to fetch tools for multi-user server ${serverId}:`, err);
			throw err;
		} finally {
			loadingByEntry[serverId] = false;
		}
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
			const rows: ToolRow[] = overrides.map((o) => {
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

				const serverMap = new Map<string, MCPCatalogServer>();
				for (const component of multiUserComponents) {
					const server = servers.find((s) => s.id === component.mcpServerID);
					if (server) {
						serverMap.set(component.mcpServerID!, server);
					}
				}
				componentServers = serverMap;
			} else {
				// Unsaved composite: no catalog to fetch from
				componentEntries = [];
				componentServers = new Map<string, MCPCatalogServer>();
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

	async function handleAdd(
		mcpCatalogEntryIds: string[],
		mcpServerIds?: string[],
		_otherSelectors?: string[]
	) {
		if (!config) {
			config = { componentServers: [] } as unknown as
				| CompositeCatalogConfig
				| CompositeRuntimeConfig;
		}
		const existingIds = new Set((config.componentServers || []).map((c) => getComponentId(c)));

		// Add catalog entry components
		const newCatalogComponents = mcpCatalogEntryIds
			.filter((id) => !existingIds.has(id))
			.map((id) => ({
				catalogEntryID: id,
				manifest: {} as Record<string, unknown>,
				toolOverrides: [],
				disabled: false
			}));

		// Add multi-user server components
		const newServerComponents =
			mcpServerIds
				?.filter((id) => !existingIds.has(id))
				.map((id) => ({
					mcpServerID: id,
					toolOverrides: [],
					disabled: false
				})) || [];

		const allNewComponents = [...newCatalogComponents, ...newServerComponents];
		if (allNewComponents.length === 0) return;

		config.componentServers = [
			...(config.componentServers || []),
			...allNewComponents
		] as unknown as typeof config.componentServers;

		await loadComponentEntries();
	}

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
		{:else if componentEntries.length > 0}
			{#each componentEntries as entry (entry.id)}
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
							onclick={() => (expanded[entry.id] = !expanded[entry.id])}
							aria-label={expanded[entry.id] ? 'Collapse' : 'Expand'}
						>
							{#if expanded[entry.id]}
								<ChevronUp class="size-4" />
							{:else}
								<ChevronDown class="size-4" />
							{/if}
						</button>
						{#if !readonly}
							<button
								type="button"
								onclick={() => removeServer(entry.id)}
								class="text-red-500 hover:text-red-700"
							>
								<Trash2 class="size-4" />
							</button>
						{/if}
					</div>
					{#if expanded[entry.id]}
						<div class="border-t border-gray-200 p-3">
							<div class="flex items-center justify-center pb-2">
								{#if !populatedByEntry[entry.id]}
									<button
										type="button"
										class="button-primary text-xs"
										disabled={loadingByEntry[entry.id]}
										onclick={async () => {
											// Launch a temporary instance and fetch tool previews, with OAuth/config when required
											if (hasEditableConfiguration(entry)) {
												configuringEntry = entry;
												initConfigureForm(entry);
												configDialog?.open();
												return;
											}
											await runPreview(entry, { config: {}, url: '' });
										}}
									>
										{#if loadingByEntry[entry.id]}
											<LoaderCircle class="size-4 animate-spin" />
										{:else}
											Populate Tools
										{/if}
									</button>
								{/if}
							</div>
							{#if toolsByEntry[entry.id]?.length}
								<div class="flex flex-col gap-2">
									{#each toolsByEntry[entry.id] as tool (tool.id)}
										<div
											class="dark:bg-surface2 dark:border-surface3 rounded border border-gray-200 bg-white p-2"
										>
											<div class="flex items-center gap-2">
												<input
													class="text-input-filled flex-1 text-sm"
													bind:value={tool.overrideName}
													oninput={() => updateCompositeToolMappings()}
													placeholder="Tool name"
												/>
												<label class="flex items-center gap-1 text-xs whitespace-nowrap">
													<input
														type="checkbox"
														bind:checked={tool.enabled}
														onchange={() => updateCompositeToolMappings()}
													/> Enable
												</label>
											</div>
											<textarea
												class="text-input-filled resize-none text-xs"
												bind:value={tool.overrideDescription}
												oninput={() => updateCompositeToolMappings()}
												placeholder="Tool description"
												rows="2"
											></textarea>
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

		{#if config?.componentServers}
			{@const multiUserComponents = config.componentServers.filter((c) => c.mcpServerID)}
			{#if multiUserComponents.length > 0}
				{#each multiUserComponents as component (component.mcpServerID)}
					<div
						class="dark:bg-surface2 dark:border-surface3 rounded-lg border border-gray-200 bg-gray-50"
					>
						<div class="flex items-center gap-3 p-3">
							<Server class="size-8 text-blue-500" />
							<div class="flex-1">
								<div class="font-medium">
									{componentServers.get(component.mcpServerID!)?.manifest?.name ||
										component.mcpServerID}
								</div>
							</div>
							<button
								type="button"
								class="icon-button"
								onclick={() =>
									(expanded[component.mcpServerID!] = !expanded[component.mcpServerID!])}
								aria-label={expanded[component.mcpServerID!] ? 'Collapse' : 'Expand'}
							>
								{#if expanded[component.mcpServerID!]}
									<ChevronUp class="size-4" />
								{:else}
									<ChevronDown class="size-4" />
								{/if}
							</button>
							{#if !readonly}
								<button
									type="button"
									onclick={() => removeServer(component.mcpServerID || '')}
									class="text-red-500 hover:text-red-700"
								>
									<Trash2 class="size-4" />
								</button>
							{/if}
						</div>
						{#if expanded[component.mcpServerID!]}
							<div class="border-t border-gray-200 p-3">
								<div class="flex items-center justify-center pb-2">
									{#if !populatedByEntry[component.mcpServerID!]}
										<button
											type="button"
											class="button-primary text-xs"
											disabled={loadingByEntry[component.mcpServerID!]}
											onclick={async () => {
												await populateMultiUserServerTools(component.mcpServerID!);
											}}
										>
											{#if loadingByEntry[component.mcpServerID!]}
												<LoaderCircle class="size-4 animate-spin" />
											{:else}
												Populate Tools
											{/if}
										</button>
									{/if}
								</div>
								{#if toolsByEntry[component.mcpServerID!]?.length}
									<div class="flex flex-col gap-2">
										{#each toolsByEntry[component.mcpServerID!] as tool (tool.id)}
											<div
												class="dark:bg-surface2 dark:border-surface3 rounded border border-gray-200 bg-white p-2"
											>
												<div class="flex items-center gap-2">
													<input
														class="text-input-filled flex-1 text-sm"
														bind:value={tool.overrideName}
														oninput={() => updateCompositeToolMappings()}
														placeholder="Tool name"
													/>
													<label class="flex items-center gap-1 text-xs whitespace-nowrap">
														<input
															type="checkbox"
															bind:checked={tool.enabled}
															onchange={() => updateCompositeToolMappings()}
														/> Enable
													</label>
												</div>
												<textarea
													class="text-input-filled resize-none text-xs"
													bind:value={tool.overrideDescription}
													oninput={() => updateCompositeToolMappings()}
													placeholder="Tool description"
													rows="2"
												></textarea>
											</div>
										{/each}
									</div>
								{/if}
							</div>
						{/if}
					</div>
				{/each}
			{/if}
		{/if}
	</div>

	{#if !readonly}
		<button
			type="button"
			onclick={() => searchDialog?.open()}
			class="dark:bg-surface2 dark:border-surface3 dark:hover:bg-surface3 flex items-center justify-center gap-2 rounded-lg border border-gray-200 bg-white p-2 text-sm font-medium hover:bg-gray-50"
		>
			<Plus class="size-4" />
			Add MCP Server
		</button>
	{/if}
</div>

<SearchMcpServers
	bind:this={searchDialog}
	onAdd={(mcpCatalogEntryIds, mcpServerIds, otherSelectors) =>
		handleAdd(mcpCatalogEntryIds, mcpServerIds, otherSelectors)}
	exclude={['*', 'default', ...(config?.componentServers ?? []).map((c) => getComponentId(c))]}
	type="filter"
	{mcpEntriesContextFn}
/>

<!-- Inline configuration dialog for previewing tools on components that require config -->
<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	name={configuringEntry?.manifest?.name}
	icon={configuringEntry?.manifest?.icon}
	submitText="Continue"
	onSave={async () => {
		const configValues = convertEnvHeadersToRecord(configureForm?.envs, configureForm?.headers);
		await runPreview(configuringEntry!, { config: configValues, url: configureForm?.url });
		configDialog?.close();
	}}
	onCancel={() => configDialog?.close()}
	onClose={() => (configuringEntry = undefined)}
	loading={false}
	error={undefined}
	isNew
	disableOutsideClick
	animate="slide"
/>
