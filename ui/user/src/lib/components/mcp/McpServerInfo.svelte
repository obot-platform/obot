<script lang="ts">
	import {
		type MCPServerTool,
		type MCPCatalogServer,
		type MCPServerPrompt,
		type McpServerResource,
		ChatService
	} from '$lib/services';
	import type { MCPCatalogEntry } from '$lib/services/admin/types';
	import { CircleCheckBig, CircleOff, LoaderCircle } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import McpServerTools from './McpServerTools.svelte';
	import { formatTimeAgo } from '$lib/time';
	import { responsive } from '$lib/stores';

	interface Props {
		class?: string;
		entry: MCPCatalogEntry | MCPCatalogServer;
	}

	function convertEntryDetails(entry: MCPCatalogEntry | MCPCatalogServer) {
		let items: Record<string, { label: string; value: string | string[] }> = {};
		if ('manifest' in entry) {
			items = {
				requiredConfig: {
					label: 'Required Config',
					value: entry.manifest?.env?.map((e) => e.key).join(', ') ?? []
				},
				users: {
					label: 'Users',
					value: '-'
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
					value: '-'
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
					label: 'Required Config',
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
					value: manifest?.repoURL ?? ''
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

		return responsive.isMobile
			? [
					items.requiredConfig,
					items.moreInformation,
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
	}

	let { entry, class: klass }: Props = $props();
	let tools = $state<MCPServerTool[]>([]);
	let prompts = $state<MCPServerPrompt[]>([]);
	let resources = $state<McpServerResource[]>([]);
	let details = $derived(convertEntryDetails(entry));
	let loading = $state(false);

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

	$effect(() => {
		if (entry && 'manifest' in entry) {
			loadServerData();
		}
	});
</script>

<div class={twMerge('flex flex-col gap-4', klass)}>
	{#if 'manifest' in entry}
		{#if entry.manifest.description}
			<p>{entry.manifest.description}</p>
		{/if}
	{:else}
		{@const manifest = entry.commandManifest || entry.urlManifest}
		{#if manifest?.description}
			<p>{manifest.description}</p>
		{/if}
	{/if}

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

{#snippet capabilitiesSection()}
	{#if 'manifest' in entry}
		<div class="flex flex-col gap-2">
			<h4 class="text-md font-semibold">Capabilities</h4>
			<ul class="flex flex-wrap items-center gap-4">
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
			'flex w-fit items-center justify-center gap-2 rounded-full px-4 py-1 text-sm font-light',
			enabled ? 'bg-blue-200/50 dark:bg-blue-800/50' : 'bg-gray-200/50 dark:bg-gray-800/50'
		)}
	>
		{#if enabled}
			<CircleCheckBig class="size-4 text-blue-500" />
		{:else}
			<CircleOff class="size-4 text-gray-400 dark:text-gray-600" />
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
		<div class="grid grid-cols-3 gap-4">
			{#each details.filter( (d) => (Array.isArray(d.value) ? d.value.length > 0 : d.value) ) as detail}
				<div
					class="dark:bg-surface2 dark:border-surface3 rounded-md border border-transparent bg-white p-4 shadow-sm"
				>
					<p class="mb-1 text-sm font-semibold">{detail.label}</p>
					{#if typeof detail.value === 'string'}
						<p class="text-xs font-light">{detail.value}</p>
					{:else if Array.isArray(detail.value)}
						<ul class="flex flex-col gap-1">
							{#each detail.value as value}
								<li class="text-xs font-light">{value}</li>
							{/each}
						</ul>
					{:else}
						<p class="text-xs font-light">-</p>
					{/if}
				</div>
			{/each}
		</div>
	</div>
{/snippet}
