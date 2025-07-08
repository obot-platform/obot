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
	import { CircleCheckBig, CircleOff, LoaderCircle, Pencil } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import McpServerTools from './McpServerTools.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { responsive } from '$lib/stores';
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import MarkdownTextEditor from '../admin/MarkdownTextEditor.svelte';

	interface Props {
		class?: string;
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

	let { entry, class: klass, editable = false, catalogId, onUpdate }: Props = $props();
	let tools = $state<MCPServerTool[]>([]);
	let prompts = $state<MCPServerPrompt[]>([]);
	let resources = $state<McpServerResource[]>([]);
	let details = $derived(convertEntryDetails(entry));
	let loading = $state(false);
	let editDescription = $state(false);
	let previousEntryId = $state<string | undefined>(undefined);
	let description = $derived(
		('manifest' in entry
			? entry.manifest.description
			: entry.commandManifest?.description || entry.urlManifest?.description) ?? ''
	);

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
	<div class={twMerge('flex flex-col gap-4 md:w-1/2 lg:w-8/12', klass)}>
		{#if editable}
			{#if editDescription}
				<MarkdownTextEditor
					bind:value={description}
					initialFocus
					onUpdate={handleDescriptionUpdate}
					onCancel={() => (editDescription = false)}
				/>
			{:else}
				<button
					class="group relative min-h-8 w-fit max-w-full pr-8 text-left"
					onclick={() => (editDescription = true)}
				>
					{#if description}
						<div class="milkdown-content">
							{@html toHTMLFromMarkdown(description)}
						</div>
					{:else}
						<span class="text-md text-gray-400 dark:text-gray-600">Add description here...</span>
					{/if}
					<div
						class="absolute top-1.5 right-0 z-10 opacity-0 transition-opacity group-hover:opacity-100"
						use:tooltip={'Edit Content'}
					>
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
	{#if 'manifest' in entry}
		<div class="flex flex-col gap-2">
			<h4 class="text-md font-semibold">Capabilities</h4>
			<ul class="flex flex-wrap items-center gap-2">
				{@render capabiliity('Tool Catalog', tools.length > 0)}
				{@render capabiliity('Prompts', prompts.length > 0)}
				{@render capabiliity('Resources', resources.length > 0)}
			</ul>
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
	{#if tools.length > 0}
		<div class="flex flex-col gap-2">
			<McpServerTools {tools} />
		</div>
	{/if}
{/snippet}

{#snippet detailsSection()}
	<div class="flex flex-col gap-2">
		<h4 class="text-md font-semibold">Details</h4>
		<div class="flex flex-col gap-4">
			{#each details.filter( (d) => (Array.isArray(d.value) ? d.value.length > 0 : d.value) ) as detail}
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
			{#each detail.value as value}
				<li class="text-xs font-light">{value}</li>
			{/each}
		</ul>
	{:else}
		<p class="text-xs font-light">-</p>
	{/if}
{/snippet}
