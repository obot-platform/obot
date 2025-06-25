<script lang="ts">
	import type { MCPCatalogServer } from '$lib/services';
	import type { MCPCatalogEntry } from '$lib/services/admin/types';
	import { twMerge } from 'tailwind-merge';
	import McpServerEntry from '../mcp/McpServerEntry.svelte';
	import CatalogServerForm from './CatalogServerForm.svelte';
	import Table from '../Table.svelte';
	import { GlobeLock, Router, Users } from 'lucide-svelte';

	interface Props {
		catalogId?: string;
		entry?: MCPCatalogEntry | MCPCatalogServer;
		type?: 'single' | 'multi' | 'remote';
		readonly?: boolean;
		onCancel?: () => void;
		onSubmit?: () => void;
	}

	let { entry, catalogId, type, readonly, onCancel, onSubmit }: Props = $props();
	const tabs = $derived(
		entry
			? [
					{ label: 'Overview', view: 'overview' },
					{ label: 'Configuration', view: 'configuration' },
					{ label: 'Access Control', view: 'access-control' },
					{ label: 'Usage', view: 'usage' },
					{ label: 'Server Instances', view: 'server-instances' }
				]
			: []
	);

	let rules = $state([]);
	let usage = $state([]);
	let instances = $state([]);

	let view = $state<string>(entry ? 'overview' : 'configuration');
</script>

<div class="flex h-full w-full flex-col gap-8">
	{#if entry}
		<h1 class="text-2xl font-semibold capitalize">
			{#if 'manifest' in entry}
				{entry.manifest.name || 'Unknown'}
			{:else}
				{entry?.commandManifest?.name || entry?.urlManifest?.name || 'Unknown'}
			{/if}
		</h1>
	{/if}
	<div class="flex flex-col gap-2">
		{#if tabs.length > 0}
			<div class="grid grid-cols-5 items-center gap-2 text-sm font-light">
				{#each tabs as tab}
					<button
						onclick={() => (view = tab.view)}
						class={twMerge(
							'rounded-md border border-transparent px-4 py-2 text-center transition-colors duration-300',
							view === tab.view && 'dark:bg-surface1 dark:border-surface3 bg-white shadow-sm',
							view !== tab.view && 'hover:bg-surface3'
						)}
					>
						{tab.label}
					</button>
				{/each}
			</div>
		{/if}

		{#if view === 'overview' && entry}
			<McpServerEntry
				class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-8 rounded-lg border border-transparent bg-white p-4 shadow-sm"
				{entry}
			/>
		{:else if view === 'configuration'}
			{@render configurationView()}
		{:else if view === 'access-control'}
			{@render accessControlView()}
		{:else if view === 'usage'}
			{@render usageView()}
		{:else if view === 'server-instances'}
			{@render serverInstancesView()}
		{/if}
	</div>
</div>

{#snippet configurationView()}
	<div class="flex flex-col gap-8">
		<CatalogServerForm {entry} {type} {readonly} {catalogId} {onCancel} {onSubmit} />
	</div>
{/snippet}

{#snippet accessControlView()}
	{#if rules.length === 0}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<GlobeLock class="size-24 text-gray-200 dark:text-gray-900" />
			<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
				No access control rules
			</h4>
			<p class="text-sm font-light text-gray-400 dark:text-gray-600">
				This server is not tied to any access control rules.
			</p>
		</div>
	{:else}
		<Table data={[]} fields={['name']} />
	{/if}
{/snippet}

{#snippet usageView()}
	{#if usage.length === 0}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<Users class="size-24 text-gray-200 dark:text-gray-900" />
			<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No usage data</h4>
			<p class="text-sm font-light text-gray-400 dark:text-gray-600">
				This server has not been used yet or data is not available.
			</p>
		</div>
	{:else}
		<Table data={[]} fields={['name']} />
	{/if}
{/snippet}

{#snippet serverInstancesView()}
	{#if instances.length === 0}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<Router class="size-24 text-gray-200 dark:text-gray-900" />
			<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No server instance</h4>
			<p class="text-sm font-light text-gray-400 dark:text-gray-600">
				No server instances have been created yet for this server.
			</p>
		</div>
	{:else}
		<Table data={[]} fields={['name']} />
	{/if}
{/snippet}
