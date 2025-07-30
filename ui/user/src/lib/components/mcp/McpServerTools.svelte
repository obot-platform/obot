<script lang="ts">
	import {
		AdminService,
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerTool,
		type Project,
		type ProjectMCP
	} from '$lib/services';
	import { ChevronDown, ChevronUp, Info, LoaderCircle, RefreshCcw, Wrench } from 'lucide-svelte';
	import Toggle from '../Toggle.svelte';
	import { slide } from 'svelte/transition';
	import { responsive } from '$lib/stores';
	import Search from '../Search.svelte';

	interface Props {
		entry: MCPCatalogEntry | MCPCatalogServer | ProjectMCP;
		catalogId?: string;
		onAuthenticate?: () => void;
		onProjectToolsUpdate?: (selected: string[]) => void;
		project?: Project;
	}

	let { entry, onAuthenticate, onProjectToolsUpdate, project }: Props = $props();
	let search = $state('');
	let tools = $state<MCPServerTool[]>([]);
	let previewTools = $derived(getToolPreview(entry));
	let loading = $state(false);
	let previousEntryId = $state<string | undefined>(undefined);
	let oauthURL = $state<string>('');
	let showRefresh = $state(false);
	// Create AbortController for cancelling API calls
	let abortController = $state<AbortController | null>(null);

	let selected = $state<string[]>([]);
	let allToolsEnabled = $derived(selected[0] === '*' || selected.length === tools.length);
	let expandedDescriptions = $state<Record<string, boolean>>({});
	let expandedParams = $state<Record<string, boolean>>({});
	let allDescriptionsEnabled = $state(true);
	let allParamsEnabled = $state(false);

	// Determine if we have "real" tools or should show previews
	let hasConnectedServer = $derived('manifest' in entry || 'mcpID' in entry);
	let showRealTools = $derived(hasConnectedServer && tools.length > 0);
	let showPreviewTools = $derived(
		previewTools.length > 0 && (!hasConnectedServer || (loading && tools.length === 0))
	);
	let displayTools = $derived(
		(showRealTools ? tools : showPreviewTools ? previewTools : []).filter(
			(tool) =>
				tool.name.toLowerCase().includes(search.toLowerCase()) ||
				tool.description?.toLowerCase().includes(search.toLowerCase())
		)
	);

	// Extract tool previews from the appropriate manifest
	function getToolPreview(entry: MCPCatalogEntry | MCPCatalogServer | ProjectMCP): MCPServerTool[] {
		if ('manifest' in entry) {
			// Connected server - get from manifest.toolPreview
			return entry.manifest?.toolPreview || [];
		} else if ('commandManifest' in entry || 'urlManifest' in entry) {
			// Catalog entry - get from commandManifest or urlManifest
			const manifest = entry.commandManifest || entry.urlManifest;
			return manifest?.toolPreview || [];
		}
		return [];
	}

	function handleToggleDescription(toolId: string) {
		if (allDescriptionsEnabled) {
			allDescriptionsEnabled = false;
			for (const { id: refToolId } of tools) {
				if (toolId !== refToolId) {
					expandedDescriptions[refToolId] = true;
				}
			}
			expandedDescriptions[toolId] = false;
		} else {
			expandedDescriptions[toolId] = !expandedDescriptions[toolId];
		}

		const expandedDescriptionValues = Object.values(expandedDescriptions);
		if (
			expandedDescriptionValues.length === tools.length &&
			expandedDescriptionValues.every((v) => v)
		) {
			allDescriptionsEnabled = true;
		}
	}

	$effect(() => {
		if (
			entry &&
			('manifest' in entry || 'mcpID' in entry) &&
			(!previousEntryId || entry.id !== previousEntryId)
		) {
			previousEntryId = entry.id;
			loadServerData();
		}
	});

	async function loadServerData() {
		// Cancel any existing requests
		if (abortController) {
			abortController.abort();
		}

		// Create new AbortController for this request
		abortController = new AbortController();

		loading = true;
		oauthURL = '';
		showRefresh = false;

		try {
			if (project) {
				oauthURL = await ChatService.getProjectMcpServerOauthURL(
					project.assistantID,
					project.id,
					entry.id,
					{
						signal: abortController.signal
					}
				);
			} else if ('sharedWithinCatalogName' in entry) {
				oauthURL = await AdminService.getMCPCatalogServerOAuthURL(
					entry.sharedWithinCatalogName,
					entry.id,
					{
						signal: abortController.signal
					}
				);
			} else {
				oauthURL = await ChatService.getMcpServerOauthURL(entry.id, {
					signal: abortController.signal
				});
			}
			if (oauthURL) {
				loading = false;
				return;
			}

			// Make a best effort attempt to load tools, prompts, and resources concurrently
			let toolCall = project
				? ChatService.listProjectMCPServerTools(project.assistantID, project.id, entry.id, {
						signal: abortController.signal
					})
				: ChatService.listMcpCatalogServerTools(entry.id, { signal: abortController.signal });

			tools = await toolCall;
			selected = tools.filter((t) => t.enabled).map((t) => t.id);
		} catch (err: unknown) {
			// Only handle errors if the request wasn't aborted
			if (err instanceof Error && err.name !== 'AbortError') {
				console.error(err);
			}
		} finally {
			loading = false;
		}
	}

	async function handleProjectToolsUpdate() {
		if (!project) return;

		try {
			await ChatService.configureProjectMcpServerTools(
				project.assistantID,
				project.id,
				entry.id,
				selected
			);
		} catch (err) {
			console.error(err);
		} finally {
			onProjectToolsUpdate?.(selected);
		}
	}
</script>

<div class="flex w-full flex-col gap-4">
	<div class="flex w-full flex-col items-center gap-2 md:flex-row">
		{#if oauthURL}
			<div class="notification-info flex w-full flex-row justify-between p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6 flex-shrink-0" />
					<p>For detailed information about this MCP server, server authentication is required.</p>
				</div>
				{#if showRefresh}
					<button
						class="button-primary flex items-center justify-center gap-1 text-center text-sm"
						onclick={async () => {
							await loadServerData();
							onAuthenticate?.();
						}}
						disabled={loading}
					>
						<RefreshCcw class="size-4 text-white" /> Reload
					</button>
				{:else}
					<a
						target="_blank"
						href={oauthURL}
						class="button-primary text-center text-sm"
						onclick={() => {
							setTimeout(() => {
								showRefresh = true;
							}, 500);
						}}
					>
						Authenticate
					</a>
				{/if}
			</div>
		{:else if showPreviewTools}
			<div class="notification-info w-full p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6" />
					<div>
						This is a preview of the tools that are available for this MCP server; the actual tools
						may differ on user connection.
					</div>
				</div>
			</div>
		{/if}
	</div>

	<div class="flex w-full flex-col gap-2">
		<div class="mb-2 flex w-full flex-col justify-between gap-4">
			<div class="flex flex-wrap items-center justify-end gap-2 md:flex-shrink-0">
				<Toggle
					checked={allDescriptionsEnabled}
					onChange={(checked) => {
						allDescriptionsEnabled = checked;
						expandedDescriptions = {};
					}}
					label="All Descriptions"
					labelInline
					classes={{
						label: 'text-sm gap-2'
					}}
				/>

				{#if !responsive.isMobile}
					<div class="bg-surface3 mx-2 h-5 w-0.5"></div>
				{/if}

				<Toggle
					checked={allParamsEnabled}
					onChange={(checked) => {
						allParamsEnabled = checked;
						expandedParams = {};
					}}
					label="All Parameters"
					labelInline
					classes={{
						label: 'text-sm gap-2'
					}}
				/>

				{#if project}
					{#if !responsive.isMobile}
						<div class="bg-surface3 mx-2 h-5 w-0.5"></div>
					{/if}

					<Toggle
						checked={allToolsEnabled}
						onChange={(checked) => {
							selected = checked ? ['*'] : [];
						}}
						label="Enable All Tools"
						labelInline
						classes={{
							label: 'text-sm gap-2'
						}}
					/>
				{/if}
			</div>

			<Search
				class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
				onChange={(val) => (search = val)}
				placeholder="Search tools..."
			/>
		</div>
		<div class="flex flex-col gap-4 overflow-hidden">
			{#if loading}
				<div class="flex items-center justify-center">
					<LoaderCircle class="size-6 animate-spin" />
				</div>
			{:else if displayTools.length > 0}
				{#each displayTools as tool (tool.name)}
					<div
						class="border-surface2 dark:bg-surface1 dark:border-surface3 flex flex-col gap-2 rounded-md border bg-white p-3 shadow-sm"
						class:pb-2={!expandedDescriptions[tool.id] && !allDescriptionsEnabled}
					>
						<div class="flex items-center justify-between gap-2">
							<p class="text-md font-semibold">
								{tool.name}
								{#if tool.unsupported}
									<span class="ml-3 text-sm text-gray-500">
										⚠️ Not yet fully supported in Obot
									</span>
								{/if}
							</p>
							<div class="flex flex-shrink-0 items-center gap-2">
								<button
									class="icon-button h-fit min-h-auto w-fit min-w-auto flex-shrink-0 p-1"
									onclick={() => handleToggleDescription(tool.id)}
								>
									{#if expandedDescriptions[tool.id]}
										<ChevronUp class="size-4" />
									{:else}
										<ChevronDown class="size-4" />
									{/if}
								</button>
								<Toggle
									checked={selected.includes(tool.id) || allToolsEnabled}
									onChange={(checked) => {
										if (allToolsEnabled) {
											selected = tools.map((t) => t.id).filter((id) => id !== tool.id);
										} else {
											selected = checked
												? [...selected, tool.id]
												: selected.filter((id) => id !== tool.id);
										}
									}}
									label="On/Off"
									disablePortal
								/>
							</div>
						</div>
						{#if expandedDescriptions[tool.id] || allDescriptionsEnabled}
							<p in:slide={{ axis: 'y' }} class="text-sm font-light text-gray-500">
								{tool.description}
							</p>
							{#if Object.keys(tool.params ?? {}).length > 0}
								{#if expandedParams[tool.id] || allParamsEnabled}
									<div
										class="from-surface2 dark:from-surface3 flex w-full flex-shrink-0 bg-linear-to-r to-transparent px-4 py-2 text-xs font-semibold text-gray-500 md:w-sm"
									>
										Parameters
									</div>
									<div class="flex flex-col px-4 text-xs" in:slide={{ axis: 'y' }}>
										<div class="flex flex-col gap-2">
											{#each Object.keys(tool.params ?? {}) as paramKey (paramKey)}
												<div class="flex flex-col items-center gap-2 md:flex-row">
													<p class="self-start font-semibold text-gray-500 md:min-w-xs">
														{paramKey}
													</p>
													<p class="self-start font-light text-gray-500">
														{tool.params?.[paramKey]}
													</p>
												</div>
											{/each}
										</div>
									</div>
								{/if}
							{/if}
						{/if}
					</div>
				{/each}
			{:else}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Wrench class="size-24 text-gray-200 dark:text-gray-900" />
					<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No tools</h4>
					<p class="text-sm font-light text-gray-400 dark:text-gray-600">
						{#if !entry || 'manifest' in entry}
							Looks like this MCP server doesn't have any tools available.
						{:else}
							Connection to to the server is required to list available tools.
						{/if}
					</p>
				</div>
			{/if}
		</div>
	</div>
</div>

{#if project}
	<div
		class="sticky bottom-0 left-0 flex w-full justify-end bg-gray-50 py-4 md:px-4 dark:bg-inherit"
	>
		<button class="button-primary flex items-center gap-1" onclick={handleProjectToolsUpdate}>
			Save
		</button>
	</div>
{/if}
