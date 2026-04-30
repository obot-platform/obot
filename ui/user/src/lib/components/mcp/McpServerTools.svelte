<script lang="ts">
	import { browser } from '$app/environment';
	import Loading from '$lib/icons/Loading.svelte';
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';
	import {
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type MCPServerTool,
		type Project,
		type ProjectMCP
	} from '$lib/services';
	import { conflictIssue, duplicateToolNames, toolNameIssue } from '$lib/services/chat/mcp';
	import { responsive } from '$lib/stores';
	import Search from '../Search.svelte';
	import Toggle from '../Toggle.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import McpOauth from './McpOauth.svelte';
	import ToolNameIssueIcon from './ToolNameIssueIcon.svelte';
	import { CircleAlert, ChevronDown, ChevronUp, Info, Wrench } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		entry: MCPCatalogEntry | MCPCatalogServer | ProjectMCP;
		onAuthenticate?: () => void;
		onProjectToolsUpdate?: (selected: string[]) => void;
		project?: Project;
		noToolsContent?: Snippet;
		classes?: {
			root?: string;
		};
		// When true, surface inline warning/error indicators next to each tool
		// name for names that may be problematic for MCP clients / inference
		// APIs (length, disallowed chars, or duplicates in this list).
		showToolNameIssues?: boolean;
	}

	let {
		entry,
		onAuthenticate,
		onProjectToolsUpdate,
		project,
		noToolsContent,
		classes,
		showToolNameIssues = false
	}: Props = $props();
	let search = $state('');
	let tools = $state<MCPServerTool[]>([]);
	let previewTools = $derived(getToolPreview(entry));
	let loading = $state(false);
	let previousEntryId = $state<string | undefined>(undefined);
	let error = $state('');

	let selected = $state<string[]>([]);
	let allToolsEnabled = $derived(selected[0] === '*' || selected.length === tools.length);
	let expanded = $state<Record<string, boolean>>({});
	let allDescriptionsEnabled = $state(false);
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

	// Detect duplicate effective names across the aggregated tool list (composite
	// previews/live lists only). Disabled via showToolNameIssues=false for other
	// contexts so the icons stay opt-in.
	let toolNameDuplicates = $derived(
		showToolNameIssues ? duplicateToolNames(displayTools.map((t) => t.name)) : new Set<string>()
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
		if (allDescriptionsEnabled && !show) {
			allDescriptionsEnabled = false;
			for (const { id: refToolId } of displayTools) {
				if (toolId !== refToolId) {
					expanded[refToolId] = true;
				}
			}
		}

		expanded[toolId] = show;
		const expandedValues = Object.values(expanded);
		if (expandedValues.length === displayTools.length && expandedValues.every((v) => v)) {
			allDescriptionsEnabled = true;
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

<div class={twMerge('flex w-full flex-col gap-4', classes?.root)}>
	<div class="flex w-full flex-col items-center gap-2 md:flex-row">
		{#if showPreviewTools}
			<div class="notification-info w-full p-3 text-sm font-light">
				<div class="flex items-center gap-3">
					<Info class="size-6 shrink-0" />
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
				<CircleAlert class="size-4" />
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
			<div class="flex flex-wrap items-center justify-end gap-2 md:shrink-0">
				<Toggle
					checked={allDescriptionsEnabled}
					onChange={(checked) => {
						allDescriptionsEnabled = checked;
						expanded = {};
					}}
					label="Show All Descriptions"
					labelInline
					classes={{
						label: 'text-sm gap-2'
					}}
				/>

				{#if project}
					{#if !responsive.isMobile}
						<div class="bg-base-400 mx-2 h-5 w-0.5"></div>
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
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
				onChange={(val) => (search = val)}
				placeholder="Search tools..."
			/>
		</div>
		<div class="flex flex-col gap-4 overflow-hidden">
			{#if loading}
				<div class="flex items-center justify-center">
					<Loading class="size-6" />
				</div>
			{:else if displayTools.length > 0}
				{#each displayTools as tool, index (`${tool.name}-${index}`)}
					{@const hasContentDisplayed = allDescriptionsEnabled || expanded[tool.id]}
					<div
						class="border-base-200 dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-2 rounded-md border p-3 shadow-sm"
						class:pb-2={hasContentDisplayed}
					>
						<div class="flex items-center justify-between gap-2">
							<p class="text-md flex min-w-0 flex-1 items-center gap-1.5 font-semibold">
								<span class="min-w-0 flex-1 truncate" title={tool.name}>{tool.name}</span>
								{#if showToolNameIssues}
									{@const conflict = conflictIssue(tool.name, toolNameDuplicates)}
									<ToolNameIssueIcon issue={conflict ?? toolNameIssue(tool.name)} />
								{/if}
								{#if tool.unsupported}
									<span class="text-muted-content ml-3 shrink-0 text-sm">
										⚠️ Not yet fully supported in Obot
									</span>
								{/if}
							</p>
							<div class="flex shrink-0 items-center gap-2">
								<IconButton
									class="btn-sm"
									onclick={() => handleToggleDescription(tool.id, !hasContentDisplayed)}
								>
									{#if hasContentDisplayed}
										<ChevronUp class="size-4" />
									{:else}
										<ChevronDown class="size-4" />
									{/if}
								</IconButton>
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
							{#if browser}
								<div
									in:slide={{ axis: 'y' }}
									class="milkdown-content text-muted-content max-w-none text-sm font-light"
								>
									{@html toHTMLFromMarkdownWithNewTabLinks(tool.description || '', true)}
								</div>
							{/if}
							{#if Object.keys(tool.params ?? {}).length > 0}
								<div
									class="from-base-300 dark:from-base-400 text-muted-content flex w-full shrink-0 bg-linear-to-r to-transparent px-4 py-2 text-xs font-semibold md:w-sm"
								>
									Parameters
								</div>
								<div class="flex flex-col px-4 text-xs" in:slide={{ axis: 'y' }}>
									<div class="flex flex-col gap-2">
										{#each Object.keys(tool.params ?? {}) as paramKey (paramKey)}
											<div class="flex flex-col items-center gap-2 md:flex-row">
												<p class="text-muted-content self-start font-semibold md:min-w-xs">
													{paramKey}
												</p>
												<p class="text-muted-content self-start font-light">
													{tool.params?.[paramKey]}
												</p>
											</div>
										{/each}
									</div>
								</div>
							{/if}
						{/if}
					</div>
				{/each}
			{:else if noToolsContent}
				{@render noToolsContent()}
			{:else}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Wrench class="text-muted-content size-24 opacity-50" />
					<h4 class="text-muted-content text-lg font-semibold">No tools</h4>
					<p class="text-muted-content text-sm font-light">
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

<div class="flex grow"></div>

{#if project && !loading && !error}
	<div class="sticky bottom-0 left-0 flex w-full justify-end bg-inherit py-4 md:px-4">
		<button class="btn btn-primary flex items-center gap-1" onclick={handleProjectToolsUpdate}>
			Save
		</button>
	</div>
{/if}
