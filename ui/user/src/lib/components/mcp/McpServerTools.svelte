<script lang="ts">
	import {
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerTool,
		type Project,
		type ProjectMCP
	} from '$lib/services';
	import { AlertCircle, ChevronDown, ChevronUp, Info, LoaderCircle, Wrench } from 'lucide-svelte';
	import Toggle from '../Toggle.svelte';
	import { slide } from 'svelte/transition';
	import { responsive } from '$lib/stores';
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';
	import Search from '../Search.svelte';
	import { browser } from '$app/environment';
	import McpOauth from './McpOauth.svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		entry: MCPCatalogEntry | MCPCatalogServer | ProjectMCP;
		onAuthenticate?: () => void;
		onProjectToolsUpdate?: (selected: string[]) => void;
		project?: Project;
		noToolsContent?: Snippet;
	}

	let { entry, onAuthenticate, onProjectToolsUpdate, project, noToolsContent }: Props = $props();
	let search = $state('');
	let tools = $state<MCPServerTool[]>([]);
	let previewTools = $derived(getToolPreview(entry));
	let loading = $state(false);
	let previousEntryId = $state<string | undefined>(undefined);
	let error = $state('');

	let selected = $state<string[]>([]);
	let allToolsEnabled = $derived(selected[0] === '*' || selected.length === tools.length);
	let expanded = $state<Record<string, boolean>>({});
	let descriptionsEnabled = $state(false);
	let paramsEnabled = $state(false);
	let showAllEnabled = $state(false);
	let abortController = $state<AbortController | null>(null);

	// Determine if we have "real" tools or should show previews
	let hasConnectedServer = $derived(
		'mcpCatalogID' in entry || 'connectURL' in entry || 'mcpID' in entry
	);
	let showRealTools = $derived(hasConnectedServer && tools.length > 0);
	let showPreviewTools = $derived(
		previewTools.length > 0 && (!hasConnectedServer || (loading && tools.length === 0))
	);
	let displayTools = $derived(
		(showRealTools
			? tools
			: showPreviewTools
				? previewTools.map((t) => ({ ...t, id: t.id || t.name }))
				: []
		).filter(
			(tool) =>
				tool.name.toLowerCase().includes(search.toLowerCase()) ||
				tool.description?.toLowerCase().includes(search.toLowerCase())
		)
	);

	// Extract tool previews from the appropriate manifest
	function getToolPreview(entry: MCPCatalogEntry | MCPCatalogServer | ProjectMCP): MCPServerTool[] {
		if ('manifest' in entry) {
			// Catalog entry or connected server - get from manifest.toolPreview
			return entry.manifest?.toolPreview || [];
		}
		return [];
	}

	function handleToggleDescription(toolId: string, show: boolean) {
		if (showAllEnabled && !show) {
			showAllEnabled = false;
			for (const { id: refToolId } of displayTools) {
				if (toolId !== refToolId) {
					expanded[refToolId] = true;
				}
			}
		}

		if (show && !descriptionsEnabled && !paramsEnabled) {
			descriptionsEnabled = true;
			paramsEnabled = true;
		}

		expanded[toolId] = show;
		const expandedValues = Object.values(expanded);
		if (expandedValues.length === displayTools.length && expandedValues.every((v) => v)) {
			showAllEnabled = true;
		}
	}

	async function loadTools() {
		// Cancel any existing requests
		if (abortController) {
			abortController.abort();
		}

		// Create new AbortController for this request
		abortController = new AbortController();
		loading = true;
		try {
			// Make a best effort attempt to load tools, prompts, and resources concurrently
			let toolCall = project
				? ChatService.listProjectMCPServerTools(project.assistantID, project.id, entry.id, {
						signal: abortController.signal
					})
				: ChatService.listMcpCatalogServerTools(entry.id, { signal: abortController.signal });
			tools = await toolCall;
			selected = tools.filter((t) => t.enabled).map((t) => t.id);
		} catch (err) {
			console.error(err);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (entry && hasConnectedServer && (!previousEntryId || entry.id !== previousEntryId)) {
			previousEntryId = entry.id;
			loadTools();
		}
	});

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

	async function handleAuthenticate() {
		await loadTools();
		onAuthenticate?.();
	}
</script>

<div class="flex w-full flex-col gap-4">
	<div class="flex w-full flex-col items-center gap-2 md:flex-row">
		{#if showPreviewTools}
			<div class="notification-info w-full p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6 flex-shrink-0" />
					<div>
						This is a preview of the tools that are available for this MCP server; the actual tools
						may differ on user connection.
					</div>
				</div>
			</div>
		{:else}
			{#key entry.id}
				<McpOauth {entry} onAuthenticate={handleAuthenticate} bind:error {project} />
			{/key}
		{/if}
		{#if error}
			<div class="notification-error flex w-full items-center gap-2 p-3">
				<AlertCircle class="size-4" />
				<div class="flex flex-col">
					<p class="text-sm font-semibold">Unable to retrieve the server's tools</p>
					<p class="text-sm font-light">
						{error}
					</p>
				</div>
			</div>
		{/if}
	</div>

	<div class="flex w-full flex-col gap-2">
		<div class="mb-2 flex w-full flex-col gap-4">
			<div class="flex flex-wrap items-center justify-end gap-2 md:flex-shrink-0">
				<Toggle
					checked={showAllEnabled}
					onChange={(checked) => {
						showAllEnabled = checked;
						if (showAllEnabled && !descriptionsEnabled && !paramsEnabled) {
							descriptionsEnabled = true;
							paramsEnabled = true;
						}

						if (!checked) {
							expanded = {};
						}
					}}
					label="Show All"
					labelInline
					classes={{
						label: 'text-sm gap-2'
					}}
				/>

				{#if !responsive.isMobile}
					<div class="bg-surface3 mx-2 h-5 w-0.5"></div>
				{/if}

				<Toggle
					checked={descriptionsEnabled}
					onChange={(checked) => {
						descriptionsEnabled = checked;

						if (!checked && !paramsEnabled && showAllEnabled) {
							showAllEnabled = false;
							expanded = {};
						}
					}}
					label="Show Descriptions"
					labelInline
					classes={{
						label: 'text-sm gap-2'
					}}
				/>

				{#if !responsive.isMobile}
					<div class="bg-surface3 mx-2 h-5 w-0.5"></div>
				{/if}

				<Toggle
					checked={paramsEnabled}
					onChange={(checked) => {
						paramsEnabled = checked;

						if (!checked && !descriptionsEnabled && showAllEnabled) {
							showAllEnabled = false;
							expanded = {};
						}
					}}
					label="Show Parameters"
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
					{@const hasContentDisplayed = showAllEnabled || expanded[tool.id]}
					<div
						class="border-surface2 dark:bg-surface1 dark:border-surface3 flex flex-col gap-2 rounded-md border bg-white p-3 shadow-sm"
						class:pb-2={hasContentDisplayed}
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
									onclick={() => handleToggleDescription(tool.id, !hasContentDisplayed)}
								>
									{#if hasContentDisplayed}
										<ChevronUp class="size-4" />
									{:else}
										<ChevronDown class="size-4" />
									{/if}
								</button>
								{#if project}
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
								{/if}
							</div>
						</div>
						{#if hasContentDisplayed}
							{#if browser && descriptionsEnabled}
								<div
									in:slide={{ axis: 'y' }}
									class="milkdown-content max-w-none text-sm font-light text-gray-500"
								>
									{@html toHTMLFromMarkdownWithNewTabLinks(tool.description || '')}
								</div>
							{/if}
							{#if Object.keys(tool.params ?? {}).length > 0}
								{#if paramsEnabled}
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
			{:else if noToolsContent}
				{@render noToolsContent()}
			{:else}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Wrench class="size-24 text-gray-200 dark:text-gray-900" />
					<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No tools</h4>
					<p class="text-sm font-light text-gray-400 dark:text-gray-600">
						{#if !entry || hasConnectedServer}
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

{#if project && !loading && !error}
	<div
		class="sticky bottom-0 left-0 flex w-full justify-end bg-gray-50 py-4 md:px-4 dark:bg-inherit"
	>
		<button class="button-primary flex items-center gap-1" onclick={handleProjectToolsUpdate}>
			Save
		</button>
	</div>
{/if}
