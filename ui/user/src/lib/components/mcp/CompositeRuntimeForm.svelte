<script lang="ts">
	import { Plus, Server, Trash2, ChevronDown, ChevronUp, LoaderCircle } from 'lucide-svelte';
	import SearchMcpServers from '../admin/SearchMcpServers.svelte';
	import { onMount } from 'svelte';
	import {
		AdminService,
		type CompositeCatalogConfig,
		type CompositeRuntimeConfig,
		type MCPCatalogEntry
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
	let expanded = $state<Record<string, boolean>>({});
	let loading = $state(false);

	type ParameterRow = {
		id: string;
		originalName: string;
		overrideName: string;
		originalDescription?: string;
		overrideDescription?: string;
	};
	type ToolRow = {
		id: string;
		originalName: string;
		overrideName: string;
		originalDescription?: string;
		overrideDescription?: string;
		enabled: boolean;
		parameters?: ParameterRow[];
	};
	let toolsByEntry = $state<Record<string, ToolRow[]>>({});
	let populatedByEntry = $state<Record<string, boolean>>({});
	let loadingByEntry = $state<Record<string, boolean>>({});

	function updateCompositeToolMappings() {
		if (!config) return;
		const componentServers = (config.componentServers || []).map((c) => {
			const rows = toolsByEntry[c.catalogEntryID] || [];
			const toolOverrides = rows.map((row) => ({
				name: row.originalName,
				overrideName: row.overrideName,
				overrideDescription: row.overrideDescription,
				enabled: row.enabled
			}));
			return { catalogEntryID: c.catalogEntryID, manifest: c.manifest, toolOverrides };
		});
		config.componentServers = componentServers as any;
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
				enabled: true,
				parameters: []
			}));
			populatedByEntry[entry.id] = true;
			updateCompositeToolMappings();
		} catch (err) {
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

	// Pre-populate toolsByEntry from existing toolOverrides in config
	function prePopulateExistingToolOverrides() {
		if (!config?.componentServers) return;

		for (const component of config.componentServers) {
			// Use the tool preview from the composite config's manifest, not the catalog entry
			if (!component.manifest?.toolPreview || !component.toolOverrides?.length) continue;

			// Build a map of tool overrides by name
			const overridesMap = new Map(
				component.toolOverrides.map((override) => [override.name, override])
			);

			// Convert tool previews to ToolRow format, applying existing overrides
			toolsByEntry[component.catalogEntryID] = component.manifest.toolPreview.map((tool) => {
				const override = overridesMap.get(tool.name);

				return {
					id: `${component.catalogEntryID}-${tool.id || tool.name}`,
					originalName: tool.name,
					overrideName: override?.overrideName || tool.name,
					originalDescription: tool.description,
					overrideDescription: override?.overrideDescription || tool.description,
					enabled: override ? (override.enabled ?? true) : true,
					parameters: []
				};
			});

			populatedByEntry[component.catalogEntryID] = true;
		}
	}

	// Load full catalog entry details for display
	async function loadComponentEntries() {
		if (!config?.componentServers) return;

		loading = true;
		try {
			if (catalogId) {
				const entries = await Promise.all(
					config.componentServers.map(async (c) => {
						try {
							return await AdminService.getMCPCatalogEntry(catalogId, c.catalogEntryID);
						} catch (e) {
							console.error(`Failed to load component entry ${c.catalogEntryID}:`, e);
							return null;
						}
					})
				);
				componentEntries = entries.filter((e): e is MCPCatalogEntry => e !== null);
			} else {
				// Composite not yet saved: resolve from context so entries show immediately
				const ctx = mcpEntriesContextFn?.();
				if (ctx?.entries?.length) {
					const ids = new Set((config.componentServers || []).map((c) => c.catalogEntryID));
					componentEntries = ctx.entries.filter((e) => ids.has(e.id));
				} else {
					componentEntries = [];
				}
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
		_mcpServerIds?: string[],
		_otherSelectors?: string[]
	) {
		if (!config) {
			config = { componentServers: [] } as any;
		}
		const existing = new Set((config.componentServers || []).map((c) => c.catalogEntryID));
		const newComponents = mcpCatalogEntryIds
			.filter((id) => !existing.has(id))
			.map((id) => ({ catalogEntryID: id, manifest: {} as any, toolOverrides: [], enabled: true }));
		if (newComponents.length === 0) return;
		config.componentServers = [...(config.componentServers || []), ...newComponents] as any;
		await loadComponentEntries();
	}

	function removeServer(entryId: string) {
		config.componentServers = (config.componentServers || []).filter(
			(c) => c.catalogEntryID !== entryId
		) as any;
		componentEntries = componentEntries.filter((e) => e.id !== entryId);
		delete toolsByEntry[entryId];
		delete populatedByEntry[entryId];
		delete loadingByEntry[entryId];
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
	exclude={['*', 'default', ...config?.componentServers?.map((c) => c.catalogEntryID)]}
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
