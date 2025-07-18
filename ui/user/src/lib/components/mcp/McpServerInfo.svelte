<script lang="ts">
	import {
		type MCPServerTool,
		type MCPCatalogServer,
		type MCPServerPrompt,
		type McpServerResource,
		ChatService,
		AdminService
	} from '$lib/services';
	import type { MCPCatalogEntry, MCPCatalogServerManifest } from '$lib/services/admin/types';
	import { CircleCheckBig, CircleOff, LoaderCircle, Pencil, TestTube } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import McpServerTools from './McpServerTools.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { responsive } from '$lib/stores';
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import MarkdownTextEditor from '../admin/MarkdownTextEditor.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import SensitiveInput from '../SensitiveInput.svelte';
	import { createProjectMcp, type MCPServerInfo } from '$lib/services/chat/mcp';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';

	interface Props {
		entry: MCPCatalogEntry | MCPCatalogServer;
		editable?: boolean;
		catalogId?: string;
		onUpdate?: () => void;
	}

	type EntryDetail = {
		label: string;
		value: string | string[];
		link?: string;
		class?: string;
		showTooltip?: boolean;
		editable?: boolean;
		catalogId?: string;
	};

	function convertEntryDetails(entry: MCPCatalogEntry | MCPCatalogServer) {
		let items: Record<string, EntryDetail> = {};
		if ('manifest' in entry) {
			items = {
				requiredConfig: {
					label: 'Required Configuration',
					value: entry.manifest?.env?.map((e) => e.key).join(', ') ?? []
				},
				users: {
					label: 'Users',
					value: ''
				},
				published: {
					label: 'Published',
					value: formatTimeAgo(entry.created).relativeTime
				},
				moreInfo: {
					label: 'More Information',
					value: ''
				},
				monthlyToolCalls: {
					label: 'Monthly Tool Calls',
					value: ''
				},
				lastUpdated: {
					label: 'Last Updated',
					value: formatTimeAgo(entry.updated).relativeTime
				}
			};
		} else {
			const manifest = entry.commandManifest || entry.urlManifest;
			items = {
				requiredConfig: {
					label: 'Required Configuration',
					value:
						manifest?.env
							?.filter((e) => e.required)
							.map((e) => e.name)
							.join(', ') ?? []
				},
				users: {
					label: 'Users',
					value: ''
				},
				published: {
					label: 'Published',
					value: formatTimeAgo(entry.created).relativeTime
				},
				moreInfo: {
					label: 'More Information',
					value: manifest?.repoURL ?? '',
					link: manifest?.repoURL ?? '',
					class: 'line-clamp-1',
					showTooltip: true
				},
				monthlyToolCalls: {
					label: 'Monthly Tool Calls',
					value: ''
				},
				lastUpdated: {
					label: 'Last Updated',
					value: ''
				}
			};
		}

		const details = responsive.isMobile
			? [
					items.requiredConfig,
					items.moreInfo,
					items.users,
					items.monthlyToolCalls,
					items.published,
					items.lastUpdated
				]
			: [
					items.requiredConfig,
					items.users,
					items.published,
					items.moreInfo,
					items.monthlyToolCalls,
					items.lastUpdated
				];
		return details.filter((d) => d);
	}

	// Extract tool previews from the appropriate manifest
	function getToolPreview(entry: MCPCatalogEntry | MCPCatalogServer): MCPServerTool[] {
		if ('manifest' in entry) {
			// Connected server - get from manifest.toolPreview
			return entry.manifest?.toolPreview || [];
		} else {
			// Catalog entry - get from commandManifest or urlManifest
			const manifest = entry.commandManifest || entry.urlManifest;
			return manifest?.toolPreview || [];
		}
	}

	let { entry, editable = false, catalogId, onUpdate }: Props = $props();
	let tools = $state<MCPServerTool[]>([]);
	let prompts = $state<MCPServerPrompt[]>([]);
	let resources = $state<McpServerResource[]>([]);
	let previewTools = $derived(getToolPreview(entry));
	let details = $derived(convertEntryDetails(entry));
	let loading = $state(false);
	let editDescription = $state(false);
	let previousEntryId = $state<string | undefined>(undefined);
	let description = $derived(
		('manifest' in entry
			? entry.manifest.description
			: entry.commandManifest?.description || entry.urlManifest?.description) ?? ''
	);

	// Test instance state
	let testInstanceLoading = $state(false);
	let testConfigDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let testConfigFields = $state<{
		envs?: MCPServerInfo['env'];
		headers?: MCPServerInfo['headers'];
		url?: string;
	}>({});

	// Determine if we have "real" tools or should show previews
	let hasConnectedServer = $derived('manifest' in entry);
	let showRealTools = $derived(hasConnectedServer && tools.length > 0);
	let showPreviewTools = $derived(
		previewTools.length > 0 && (!hasConnectedServer || (loading && tools.length === 0))
	);
	let displayTools = $derived(showRealTools ? tools : showPreviewTools ? previewTools : []);
	let isTestable = $derived(() => {
		const hasSourceURL = !!(entry as MCPCatalogEntry).sourceURL;
		const testable = !hasConnectedServer && !hasSourceURL && editable;
		console.log('isTestable debug:', {
			entryId: entry.id,
			hasConnectedServer,
			hasSourceURL,
			sourceURL: (entry as MCPCatalogEntry).sourceURL,
			editable,
			hasManifest: 'manifest' in entry,
			testable
		});
		return testable;
	});

	async function loadServerData() {
		loading = true;
		try {
			tools = await ChatService.listMcpCatalogServerTools(entry.id);
		} catch (err) {
			tools = [];
			console.error(err);
		}
		try {
			prompts = await ChatService.listMcpCatalogServerPrompts(entry.id);
		} catch (err) {
			prompts = [];
			console.error(err);
		}
		try {
			resources = await ChatService.listMcpCatalogServerResources(entry.id);
		} catch (err) {
			resources = [];
			console.error(err);
		}
		loading = false;
	}

	function hasEditableConfiguration(entry: MCPCatalogEntry): boolean {
		const manifest = entry.commandManifest ?? entry.urlManifest;
		return (manifest?.env?.length ?? 0) > 0 || (manifest?.headers?.length ?? 0) > 0 || manifest?.hostname !== undefined;
	}

	function convertEnvHeadersToRecord(envs?: MCPServerInfo['env'], headers?: MCPServerInfo['headers']): Record<string, string> {
		const result: Record<string, string> = {};
		if (envs) {
			for (const env of envs) {
				if (env.value && env.value.trim() !== '') {
					result[env.key] = env.value;
				}
			}
		}
		if (headers) {
			for (const header of headers) {
				if (header.value && header.value.trim() !== '') {
					result[header.key] = header.value;
				}
			}
		}
		return result;
	}

	async function handleTestInstance() {
		if (hasConnectedServer) return;
		
		const catalogEntry = entry as MCPCatalogEntry;
		const manifest = catalogEntry.commandManifest ?? catalogEntry.urlManifest;
		
		if (!manifest) {
			console.error('No server manifest found');
			return;
		}

		if (hasEditableConfiguration(catalogEntry)) {
			// Setup configuration fields
			const envs = (manifest?.env ?? []).map((env) => ({ ...env, value: '' }));
			const headers = (manifest?.headers ?? []).map((header) => ({ ...header, value: '' }));
			const url = manifest?.fixedURL ?? '';
			
			testConfigFields = { envs, headers, url };
			testConfigDialog?.open();
		} else {
			// No configuration needed, run test directly
			await runTestInstance();
		}
	}

	async function runTestInstance() {
		if (hasConnectedServer) return;
		
		testInstanceLoading = true;
		try {
			const catalogEntry = entry as MCPCatalogEntry;
			
			// Prepare test configuration
			const envConfig = convertEnvHeadersToRecord(testConfigFields.envs, undefined);
			const headerConfig = convertEnvHeadersToRecord(undefined, testConfigFields.headers);
			
			const testConfig = {
				env: envConfig,
				headers: headerConfig,
				...(testConfigFields.url ? { url: testConfigFields.url } : {})
			};

			// Call the new single test endpoint (also updates tool preview automatically)
			const fetchedTools = await ChatService.testMcpCatalogEntryInstance(
				catalogId || DEFAULT_MCP_CATALOG_ID,
				catalogEntry.id,
				testConfig
			);

			// Refresh the entry to show updated tool previews
			onUpdate?.();
			
			console.log(`Successfully fetched ${fetchedTools.length} tools and updated tool preview for ${catalogEntry.commandManifest?.name ?? catalogEntry.urlManifest?.name}`);
		} catch (error) {
			console.error('Failed to test MCP instance:', error);
		} finally {
			testInstanceLoading = false;
			testConfigDialog?.close();
		}
	}

	async function handleDescriptionUpdate(markdown: string) {
		if (!entry?.id || !catalogId) return;

		if ('manifest' in entry) {
			await AdminService.updateMCPCatalogServer(catalogId, entry.id, {
				...(entry.manifest as MCPCatalogServerManifest['manifest']),
				description: markdown
			});
		} else {
			const manifest = entry.commandManifest || entry.urlManifest;
			await AdminService.updateMCPCatalogEntry(catalogId, entry.id, {
				...manifest,
				description: markdown
			});
		}

		editDescription = false;
		onUpdate?.();
	}

	$effect(() => {
		if (entry && 'manifest' in entry && entry.id !== previousEntryId) {
			previousEntryId = entry.id;
			loadServerData();
		}
	});
</script>

<div class="flex w-full flex-col gap-4 md:flex-row">
	<div
		class="dark:bg-surface1 dark:border-surface3 flex h-fit flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm md:w-1/2 lg:w-8/12"
	>
		{#if editable}
			{#if editDescription}
				<MarkdownTextEditor
					bind:value={description}
					initialFocus
					onUpdate={handleDescriptionUpdate}
					onCancel={() => (editDescription = false)}
				/>
			{:else if description}
				<div class="group relative w-full">
					<div class="milkdown-content">
						{@html toHTMLFromMarkdown(description)}
					</div>
					<button
						class="icon-button absolute top-0 right-0 z-10 min-h-8 opacity-0 transition-all group-hover:opacity-100"
						onclick={() => (editDescription = true)}
					>
						<Pencil class="size-5 text-gray-400 dark:text-gray-600" />
					</button>
				</div>
			{:else}
				<button
					class="group relative flex min-h-8 w-full justify-between gap-2 pt-0 text-left"
					onclick={() => (editDescription = true)}
				>
					<span class="text-md text-gray-400 dark:text-gray-600">Add description here...</span>
					<div class="icon-button opacity-0 group-hover:opacity-100">
						<Pencil class="size-5 text-gray-400 dark:text-gray-600" />
					</div>
				</button>
			{/if}
		{:else if description}
			<div class="milkdown-content">
				{@html toHTMLFromMarkdown(description)}
			</div>
		{/if}
	</div>
	<div
		class="dark:bg-surface1 dark:border-surface3 flex h-fit w-full flex-shrink-0 flex-col gap-4 rounded-md border border-transparent bg-white p-4 shadow-sm md:w-1/2 lg:w-4/12"
	>
		{#if loading}
			<div class="flex items-center justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else}
			{@render capabilitiesSection()}
			{@render toolsSection()}
			{@render detailsSection()}
		{/if}
	</div>
</div>

{#snippet capabilitiesSection()}
	{#if hasConnectedServer}
		<div class="flex flex-col gap-2">
			<h4 class="text-md font-semibold">Capabilities</h4>
			<ul class="flex flex-wrap items-center gap-2">
				{@render capabiliity('Tool Catalog', displayTools.length > 0)}
				{@render capabiliity('Prompts', prompts.length > 0)}
				{@render capabiliity('Resources', resources.length > 0)}
			</ul>
		</div>
	{:else }
		<div class="flex flex-col gap-2">
			<div class="flex items-center justify-between">
				<h4 class="text-md font-semibold">Tools Preview</h4>
				<button
					class="flex items-center gap-1 rounded-full bg-blue-500 px-4 py-2 text-xs font-medium text-white hover:bg-blue-600 disabled:opacity-50 disabled:cursor-not-allowed"
					onclick={handleTestInstance}
					disabled={testInstanceLoading}
					use:tooltip={'Create a test instance to fetch available tools'}
				>
					{#if testInstanceLoading}
						<LoaderCircle class="size-3 animate-spin" />
						Testing...
					{:else}
						<TestTube class="size-3" />
						Test Instance
					{/if}
				</button>
			</div>
			{#if previewTools.length > 0}
				<p class="text-xs text-gray-500">
					{previewTools.length} tool{previewTools.length === 1 ? '' : 's'} available
				</p>
			{:else}
				<p class="text-xs text-gray-500">
					No tool preview available. Click "Test Instance" to fetch available tools.
				</p>
			{/if}
		</div>
	{/if}
{/snippet}

{#snippet capabiliity(name: string, enabled: boolean)}
	<li
		class={twMerge(
			'flex w-fit items-center justify-center gap-1 rounded-full px-4 py-1 text-xs font-light',
			enabled ? 'bg-blue-200/50 dark:bg-blue-800/50' : 'bg-gray-200/50 dark:bg-gray-800/50'
		)}
	>
		{#if enabled}
			<CircleCheckBig class="size-3 text-blue-500" />
		{:else}
			<CircleOff class="size-3 text-gray-400 dark:text-gray-600" />
		{/if}
		{name}
	</li>
{/snippet}

{#snippet toolsSection()}
	{#if displayTools.length > 0}
		<div class="flex flex-col gap-2">
			<div class="flex items-center gap-2">
				<h4 class="text-md font-semibold">Tools</h4>
				{#if showPreviewTools}
					<span
						class="rounded-full bg-blue-100 px-2 py-0.5 text-[10px] font-medium text-blue-700 dark:bg-blue-900 dark:text-blue-300"
					>
						Preview
					</span>
				{/if}
				{#if hasConnectedServer && loading}
					<LoaderCircle class="size-3 animate-spin text-gray-400" />
				{/if}
			</div>
			<McpServerTools tools={displayTools} />
		</div>
	{/if}
{/snippet}

{#snippet detailsSection()}
	<div class="flex flex-col gap-2">
		<h4 class="text-md font-semibold">Details</h4>
		<div class="flex flex-col gap-4">
			{#each details.filter( (d) => (Array.isArray(d.value) ? d.value.length > 0 : d.value) ) as detail, i (i)}
				<div
					class="dark:bg-surface2 dark:border-surface3 border-surface2 rounded-md border bg-gray-50 p-3"
				>
					<p class="mb-1 text-xs font-medium">{detail.label}</p>
					{#if detail.link}
						<a href={detail.link} class="text-link" target="_blank" rel="noopener noreferrer">
							{#if detail.showTooltip && typeof detail.value === 'string'}
								<span use:tooltip={detail.value}>
									{@render detailSection(detail)}
								</span>
							{:else}
								{@render detailSection(detail)}
							{/if}
						</a>
					{:else if detail.showTooltip && typeof detail.value === 'string'}
						<span use:tooltip={detail.value}>
							{@render detailSection(detail)}
						</span>
					{:else}
						{@render detailSection(detail)}
					{/if}
				</div>
			{/each}
		</div>
	</div>
{/snippet}

{#snippet detailSection(detail: EntryDetail)}
	{#if typeof detail.value === 'string'}
		<p class={twMerge('text-xs font-light', detail.class)}>{detail.value}</p>
	{:else if Array.isArray(detail.value)}
		<ul class="flex flex-col gap-1">
			{#each detail.value as value, i (i)}
				<li class="text-xs font-light">{value}</li>
			{/each}
		</ul>
	{:else}
		<p class="text-xs font-light">-</p>
	{/if}
{/snippet}

<ResponsiveDialog bind:this={testConfigDialog} animate="slide">
	{#snippet titleContent()}
		<div class="bg-surface1 rounded-sm p-1 dark:bg-gray-600">
			{#if (entry as MCPCatalogEntry).commandManifest?.icon || (entry as MCPCatalogEntry).urlManifest?.icon}
				<img 
					src={(entry as MCPCatalogEntry).commandManifest?.icon || (entry as MCPCatalogEntry).urlManifest?.icon} 
					alt={(entry as MCPCatalogEntry).commandManifest?.name || (entry as MCPCatalogEntry).urlManifest?.name} 
					class="size-8" 
				/>
			{:else}
				<TestTube class="size-8" />
			{/if}
		</div>
		Test {(entry as MCPCatalogEntry).commandManifest?.name || (entry as MCPCatalogEntry).urlManifest?.name}
	{/snippet}

	{#if testConfigFields}
		<div class="flex flex-col gap-4 p-4">
			<p class="text-sm text-gray-600">Configure the server to test and fetch available tools:</p>
			
			{#if testConfigFields.url !== undefined}
				<div class="flex flex-col gap-1">
					<label for="test-url" class="text-sm font-medium">Server URL</label>
					<input
						id="test-url"
						bind:value={testConfigFields.url}
						class="text-input-filled"
						placeholder="Enter server URL"
					/>
				</div>
			{/if}

			{#if testConfigFields.envs && testConfigFields.envs.length > 0}
				<div class="flex flex-col gap-2">
					<h4 class="text-sm font-medium">Environment Variables</h4>
					{#each testConfigFields.envs as env}
						<div class="flex flex-col gap-1">
							<label for="test-env-{env.key}" class="text-xs font-light">{env.name || env.key}</label>
							{#if env.description}
								<p class="text-xs text-gray-500">{env.description}</p>
							{/if}
							<SensitiveInput
								name="test-env-{env.key}"
								bind:value={env.value}
							/>
						</div>
					{/each}
				</div>
			{/if}

			{#if testConfigFields.headers && testConfigFields.headers.length > 0}
				<div class="flex flex-col gap-2">
					<h4 class="text-sm font-medium">Headers</h4>
					{#each testConfigFields.headers as header}
						<div class="flex flex-col gap-1">
							<label for="test-header-{header.key}" class="text-xs font-light">{header.name || header.key}</label>
							{#if header.description}
								<p class="text-xs text-gray-500">{header.description}</p>
							{/if}
							<SensitiveInput
								name="test-header-{header.key}"
								bind:value={header.value}
							/>
						</div>
					{/each}
				</div>
			{/if}

			<div class="flex justify-end gap-2">
				<button class="button" onclick={() => testConfigDialog?.close()}>Cancel</button>
				<button 
					class="button-primary flex items-center gap-1" 
					onclick={runTestInstance}
					disabled={testInstanceLoading}
				>
					{#if testInstanceLoading}
						<LoaderCircle class="size-3 animate-spin" />
						Testing...
					{:else}
						<TestTube class="size-3" />
						Test & Fetch Tools
					{/if}
				</button>
			</div>
		</div>
	{/if}
</ResponsiveDialog>
