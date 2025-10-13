<script lang="ts">
	import { Plus, Server, Trash2, ChevronDown, ChevronUp, LoaderCircle } from 'lucide-svelte';
	import SearchMcpServers from '../admin/SearchMcpServers.svelte';
	import { onMount } from 'svelte';
	import { AdminService, type MCPCatalogEntry } from '$lib/services';
	import type { AdminMcpServerAndEntriesContext } from '$lib/context/admin/mcpServerAndEntries.svelte';
	import CatalogConfigureForm, { type LaunchFormData } from './CatalogConfigureForm.svelte';
	import { hasEditableConfiguration, convertEnvHeadersToRecord } from '$lib/services/chat/mcp';

	interface Props {
		compositeConfig: {
			components: { catalogEntryName: string; toolOverrides?: any[]; promptOverrides?: any[] }[];
		};
		readonly?: boolean;
		catalogId?: string;
		mcpEntriesContextFn?: () => AdminMcpServerAndEntriesContext;
	}

	let { compositeConfig = $bindable(), readonly, catalogId, mcpEntriesContextFn }: Props = $props();
	let searchDialog = $state<ReturnType<typeof SearchMcpServers>>();
	let componentEntries = $state<MCPCatalogEntry[]>([]);
	let expanded = $state<Record<string, boolean>>({});
	let expandedToolParams = $state<Record<string, boolean>>({});
	let loading = $state(false);
	type ParameterRow = {
		id: string;
		originalName: string;
		exposedName: string;
		originalDescription?: string;
		exposedDescription?: string;
	};
	type ToolRow = {
		id: string;
		originalName: string;
		exposedName: string;
		originalDescription?: string;
		exposedDescription?: string;
		enabled: boolean;
		parameters?: ParameterRow[];
	};
	type PromptArgumentRow = {
		id: string;
		originalName: string;
		exposedName: string;
		originalDescription?: string;
		exposedDescription?: string;
	};
	type PromptRow = {
		id: string;
		originalName: string;
		exposedName: string;
		originalDescription?: string;
		exposedDescription?: string;
		enabled: boolean;
		promptArgs?: PromptArgumentRow[];
	};
	let toolsByEntry = $state<Record<string, ToolRow[]>>({});
	let promptsByEntry = $state<Record<string, PromptRow[]>>({});
	let populatedByEntry = $state<Record<string, boolean>>({});
	let loadingByEntry = $state<Record<string, boolean>>({});
	let expandedPromptArgs = $state<Record<string, boolean>>({});

	function teaser(text?: string, max = 140): string {
		if (!text) return '';
		return text.length > max ? text.slice(0, max).trimEnd() + '…' : text;
	}

	function updateCompositeToolMappings() {
		if (!compositeConfig) return;
		const components = (compositeConfig.components || []).map((c) => {
			const toolRows = toolsByEntry[c.catalogEntryName] || [];
			const toolOverrides = toolRows.map((row) => ({
				name: row.originalName,
				overrideName: row.exposedName,
				overrideDescription: row.exposedDescription,
				enabled: row.enabled,
				parameterOverrides: row.parameters?.map((p) => ({
					name: p.originalName,
					overrideName: p.exposedName,
					overrideDescription: p.exposedDescription
				}))
			}));

			const promptRows = promptsByEntry[c.catalogEntryName] || [];
			const promptOverrides = promptRows.map((row) => ({
				name: row.originalName,
				overrideName: row.exposedName,
				overrideDescription: row.exposedDescription,
				enabled: row.enabled,
				argumentOverrides: row.promptArgs?.map((a) => ({
					name: a.originalName,
					overrideName: a.exposedName,
					overrideDescription: a.exposedDescription
				}))
			}));

			return { catalogEntryName: c.catalogEntryName, toolOverrides, promptOverrides };
		});
		compositeConfig.components = components;
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
				{ preview: true }
			)) as unknown as MCPCatalogEntry;
			const preview = resp?.manifest?.toolPreview || [];
			toolsByEntry[entry.id] = preview.map((t) => {
				// Extract parameters from tool params
				const parameters: ParameterRow[] = t.params
					? Object.keys(t.params).map((paramName) => ({
							id: `${entry.id}-${t.id || t.name}-${paramName}`,
							originalName: paramName,
							exposedName: paramName,
							originalDescription: t.params?.[paramName],
							exposedDescription: t.params?.[paramName]
						}))
					: [];

				return {
					id: `${entry.id}-${t.id || t.name}`,
					originalName: t.name,
					exposedName: t.name,
					originalDescription: t.description,
					exposedDescription: t.description,
					enabled: true,
					parameters
				};
			});

			// Process prompts
			const promptPreview = resp?.manifest?.promptPreview || [];
			promptsByEntry[entry.id] = promptPreview.map((p) => {
				const promptArgs: PromptArgumentRow[] = p.arguments
					? p.arguments.map((arg) => ({
							id: `${entry.id}-${p.name}-${arg.name}`,
							originalName: arg.name,
							exposedName: arg.name,
							originalDescription: arg.description,
							exposedDescription: arg.description
						}))
					: [];

				return {
					id: `${entry.id}-${p.name}`,
					originalName: p.name,
					exposedName: p.name,
					originalDescription: p.description,
					exposedDescription: p.description,
					enabled: true,
					promptArgs
				};
			});

			populatedByEntry[entry.id] = true;
			updateCompositeToolMappings();
		} catch (err) {
			const msg = err instanceof Error ? err.message : String(err);
			if (msg.includes('OAuth')) {
				const oauthURL = await AdminService.getMcpCatalogToolPreviewsOauth(
					catalogId!,
					entry.id,
					body
				);
				if (oauthURL) window.open(oauthURL, '_blank');
			} else {
				throw err;
			}
		} finally {
			loadingByEntry[entry.id] = false;
		}
	}

	// Load full catalog entry details for display
	async function loadComponentEntries() {
		if (!compositeConfig?.components || !catalogId) return;

		loading = true;
		try {
			const entries = await Promise.all(
				compositeConfig.components.map(async (c) => {
					try {
						return await AdminService.getMCPCatalogEntry(catalogId, c.catalogEntryName);
					} catch (e) {
						console.error(`Failed to load component entry ${c.catalogEntryName}:`, e);
						return null;
					}
				})
			);
			componentEntries = entries.filter((e): e is MCPCatalogEntry => e !== null);
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		loadComponentEntries();
	});

	// Re-fetch component entry details whenever the selected component IDs or catalog change
	$effect(() => {
		const idsKey = compositeConfig?.components?.map((c) => c.catalogEntryName).join(',') || '';
		const catKey = catalogId || '';
		// touch keys so Svelte tracks them
		idsKey;
		catKey;
		loadComponentEntries();
	});

	function handleAdd(mcpCatalogEntryIds: string[]) {
		if (!compositeConfig) {
			compositeConfig = { components: [] } as any;
		}
		const existing = new Set((compositeConfig.components || []).map((c) => c.catalogEntryName));
		const newComponents = mcpCatalogEntryIds
			.filter((id) => !existing.has(id))
			.map((id) => ({ catalogEntryName: id, toolOverrides: [], promptOverrides: [] }));
		compositeConfig.components = [...(compositeConfig.components || []), ...newComponents];
	}

	function removeServer(entryId: string) {
		compositeConfig.components = (compositeConfig.components || []).filter(
			(c) => c.catalogEntryName !== entryId
		);
		delete toolsByEntry[entryId];
		delete promptsByEntry[entryId];
		delete populatedByEntry[entryId];
		delete loadingByEntry[entryId];
	}
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
>
	<h4 class="text-sm font-semibold">Component MCP Servers</h4>

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
							{#if entry.manifest?.description}
								<div class="text-sm text-gray-500 dark:text-gray-400">
									{teaser(entry.manifest.description, 160)}
								</div>
							{/if}
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
													bind:value={tool.exposedName}
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
												{#if tool.parameters && tool.parameters.length > 0}
													<button
														type="button"
														class="icon-button"
														onclick={() =>
															(expandedToolParams[tool.id] = !expandedToolParams[tool.id])}
														aria-label={expandedToolParams[tool.id]
															? 'Collapse parameters'
															: 'Expand parameters'}
													>
														{#if expandedToolParams[tool.id]}
															<ChevronUp class="size-4" />
														{:else}
															<ChevronDown class="size-4" />
														{/if}
													</button>
												{/if}
											</div>
											<textarea
												class="text-input-filled resize-none text-xs"
												bind:value={tool.exposedDescription}
												oninput={() => updateCompositeToolMappings()}
												placeholder="Tool description"
												rows="2"
											></textarea>

											{#if expandedToolParams[tool.id] && tool.parameters && tool.parameters.length > 0}
												<div class="mt-3 space-y-2 border-t border-gray-200 pt-3">
													<div class="text-xs font-semibold text-gray-700 dark:text-gray-300">
														Parameters:
													</div>
													{#each tool.parameters as param (param.id)}
														<div class="ml-4 flex flex-col gap-1">
															<input
																class="text-input-filled text-xs"
																bind:value={param.exposedName}
																oninput={() => updateCompositeToolMappings()}
																placeholder="Parameter name"
															/>
															<input
																class="text-input-filled text-xs"
																bind:value={param.exposedDescription}
																oninput={() => updateCompositeToolMappings()}
																placeholder="Parameter description (optional)"
															/>
														</div>
													{/each}
												</div>
											{/if}
										</div>
									{/each}
								</div>
							{/if}
							{#if promptsByEntry[entry.id]?.length}
								<div class="mt-4 flex flex-col gap-2">
									<h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">Prompts:</h5>
									{#each promptsByEntry[entry.id] as prompt (prompt.id)}
										<div
											class="dark:bg-surface2 dark:border-surface3 rounded border border-gray-200 bg-white p-2"
										>
											<div class="flex items-center gap-2">
												<input
													class="text-input-filled flex-1 text-sm"
													bind:value={prompt.exposedName}
													oninput={() => updateCompositeToolMappings()}
													placeholder="Prompt name"
												/>
												<label class="flex items-center gap-1 text-xs whitespace-nowrap">
													<input
														type="checkbox"
														bind:checked={prompt.enabled}
														onchange={() => updateCompositeToolMappings()}
													/> Enable
												</label>
												{#if prompt.promptArgs && prompt.promptArgs.length > 0}
													<button
														type="button"
														class="icon-button"
														onclick={() =>
															(expandedPromptArgs[prompt.id] = !expandedPromptArgs[prompt.id])}
														aria-label={expandedPromptArgs[prompt.id]
															? 'Collapse arguments'
															: 'Expand arguments'}
													>
														{#if expandedPromptArgs[prompt.id]}
															<ChevronUp class="size-4" />
														{:else}
															<ChevronDown class="size-4" />
														{/if}
													</button>
												{/if}
											</div>
											<textarea
												class="text-input-filled resize-none text-xs"
												bind:value={prompt.exposedDescription}
												oninput={() => updateCompositeToolMappings()}
												placeholder="Prompt description"
												rows="2"
											></textarea>

											{#if expandedPromptArgs[prompt.id] && prompt.promptArgs && prompt.promptArgs.length > 0}
												<div class="mt-3 space-y-2 border-t border-gray-200 pt-3">
													<div class="text-xs font-semibold text-gray-700 dark:text-gray-300">
														Arguments:
													</div>
													{#each prompt.promptArgs as arg (arg.id)}
														<div class="ml-4 flex flex-col gap-1">
															<input
																class="text-input-filled text-xs"
																bind:value={arg.exposedName}
																oninput={() => updateCompositeToolMappings()}
																placeholder="Argument name"
															/>
															<input
																class="text-input-filled text-xs"
																bind:value={arg.exposedDescription}
																oninput={() => updateCompositeToolMappings()}
																placeholder="Argument description (optional)"
															/>
														</div>
													{/each}
												</div>
											{/if}
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
				No component servers added yet. Click the button below to add servers.
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

	<p class="text-xs text-gray-500 dark:text-gray-400">
		Select one or more MCP catalog entries to combine into a single composite server. Users will see
		this as a single server with aggregated tools and resources.
	</p>
</div>

<SearchMcpServers
	bind:this={searchDialog}
	onAdd={(mcpCatalogEntryIds) => handleAdd(mcpCatalogEntryIds)}
	exclude={compositeConfig?.components?.map((c) => c.catalogEntryName)}
	type="acr"
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
