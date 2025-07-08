<script lang="ts">
	import { AdminService, type MCPCatalogServer } from '$lib/services';
	import type { AccessControlRule, MCPCatalogEntry } from '$lib/services/admin/types';
	import { twMerge } from 'tailwind-merge';
	import McpServerInfo from '../mcp/McpServerInfo.svelte';
	import CatalogServerForm from './CatalogServerForm.svelte';
	import Table from '../Table.svelte';
	import { GlobeLock, ListFilter, LoaderCircle, Router, Trash2, Users } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { goto } from '$app/navigation';
	import Confirm from '../Confirm.svelte';

	interface Props {
		catalogId?: string;
		entry?: MCPCatalogEntry | MCPCatalogServer;
		type?: 'single' | 'multi' | 'remote';
		readonly?: boolean;
		onCancel?: () => void;
		onSubmit?: () => void;
		onUpdate?: () => void;
	}

	let { entry, catalogId, type, readonly, onCancel, onSubmit, onUpdate }: Props = $props();
	const tabs = $derived(
		entry
			? [
					{ label: 'Overview', view: 'overview' },
					{ label: 'Configuration', view: 'configuration' },
					{ label: 'Access Control', view: 'access-control' },
					{ label: 'Usage', view: 'usage' },
					{ label: 'Server Instances', view: 'server-instances' },
					{ label: 'Filters', view: 'filters' }
				]
			: []
	);

	let usage = $state([]);
	let instances = $state([]);
	let listAccessControlRules = $state<Promise<AccessControlRule[]>>();
	let deleteServer = $state(false);
	let deleteResourceFromRule = $state<{
		rule: AccessControlRule;
		resourceId: string;
	}>();
	let view = $state<string>(entry ? 'overview' : 'configuration');

	$effect(() => {
		if (view === 'access-control') {
			listAccessControlRules = AdminService.listAccessControlRules();
		}
	});

	function filterRulesByEntry(rules?: AccessControlRule[]) {
		if (!entry || !rules) return [];
		return rules.filter((r) =>
			r.resources?.find((resource) => resource.id === entry.id || resource.id === '*')
		);
	}
</script>

<div class="flex h-full w-full flex-col gap-4">
	{#if entry}
		<div class="flex items-center justify-between gap-4">
			<div class="flex items-center gap-2">
				{#if 'manifest' in entry}
					{#if entry.manifest.icon}
						<img
							src={entry.manifest.icon}
							alt={entry.manifest.name}
							class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600"
						/>
					{/if}
					<h1 class="text-2xl font-semibold capitalize">{entry.manifest.name || 'Unknown'}</h1>
				{:else}
					{@const icon = entry.commandManifest?.icon || entry.urlManifest?.icon}
					{#if icon}
						<img
							src={icon}
							alt={entry.commandManifest?.name || entry.urlManifest?.name}
							class="bg-surface1 size-10 rounded-md p-1 dark:bg-gray-600"
						/>
					{/if}
					<h1 class="text-2xl font-semibold capitalize">
						{entry?.commandManifest?.name || entry?.urlManifest?.name || 'Unknown'}
					</h1>
				{/if}
			</div>
			{#if !readonly}
				<button
					class="button-destructive flex items-center gap-1 text-xs font-normal"
					use:tooltip={'Delete Server'}
					onclick={() => {
						deleteServer = true;
					}}
				>
					<Trash2 class="size-4" />
				</button>
			{/if}
		</div>
	{/if}
	<div class="flex flex-col gap-2">
		{#if tabs.length > 0}
			<div
				class="grid grid-cols-3 items-center gap-2 text-sm font-light md:grid-cols-4 lg:grid-cols-6"
			>
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
			<McpServerInfo editable={!readonly} {catalogId} {entry} {onUpdate} />
		{:else if view === 'configuration'}
			{@render configurationView()}
		{:else if view === 'access-control'}
			{@render accessControlView()}
		{:else if view === 'usage'}
			{@render usageView()}
		{:else if view === 'server-instances'}
			{@render serverInstancesView()}
		{:else if view === 'filters'}
			{@render filtersView()}
		{/if}
	</div>
</div>

{#snippet configurationView()}
	<div class="flex flex-col gap-8">
		<CatalogServerForm
			{entry}
			{type}
			{readonly}
			{catalogId}
			{onCancel}
			{onSubmit}
			hideTitle={Boolean(entry)}
		>
			{#snippet readonlyMessage()}
				{#if entry && 'sourceURL' in entry && !!entry.sourceURL}
					<p>
						This MCP Server comes from an external Git Source URL <span
							class="text-xs text-gray-500">({entry.sourceURL.split('/').pop()})</span
						> and cannot be edited.
					</p>
				{:else}
					<p>This MCP server is non-editable.</p>
				{/if}
			{/snippet}
		</CatalogServerForm>
	</div>
{/snippet}

{#snippet accessControlView()}
	{#await listAccessControlRules}
		<div class="flex w-full justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:then rules}
		{@const serverRules = entry ? filterRulesByEntry(rules) : []}
		{#if serverRules && serverRules.length > 0}
			<Table
				data={serverRules}
				fields={['displayName', 'resources']}
				headers={[
					{ title: 'Rule', property: 'displayName' },
					{ title: 'Reference', property: 'resources' }
				]}
				onSelectRow={(d) => {
					if (!entry) return;
					goto(
						`/v2/admin/access-control/${d.id}?from=${encodeURIComponent(`/mcp-servers/${entry.id}`)}`
					);
				}}
			>
				{#snippet onRenderColumn(property, d)}
					<span class="flex min-h-9 items-center">
						{#if property === 'resources'}
							{@const referencedResource = d.resources?.find(
								(r) => r.id === entry?.id || r.id === '*'
							)}
							{referencedResource?.id === '*' ? 'Everything' : 'Self'}
						{:else}
							{d[property as keyof typeof d]}
						{/if}
					</span>
				{/snippet}
			</Table>
		{:else}
			<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<GlobeLock class="size-24 text-gray-200 dark:text-gray-900" />
				<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
					No access control rules
				</h4>
				<p class="text-sm font-light text-gray-400 dark:text-gray-600">
					This server is not tied to any access control rules.
				</p>
			</div>
		{/if}
	{/await}
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

{#snippet filtersView()}
	<div class="mt-12 flex w-lg flex-col items-center gap-4 self-center text-center">
		<ListFilter class="size-24 text-gray-200 dark:text-gray-900" />
		<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">Filters</h4>
		<p class="text-md text-left font-light text-gray-400 dark:text-gray-600">
			The <b class="font-semibold">Filters</b> feature allows you to intercept and process incoming
			requests
			<b class="font-semibold">before they reach the MCP Server</b>. This enables you to perform
			critical tasks such as
			<b class="font-semibold"
				>authorization, request logging, tool access control, or traffic routing</b
			>. <br /><br />

			Filters act as customizable middleware components, giving you control over how requests are
			handled and whether they should be modified, allowed, or blocked before reaching the core
			application logic.
		</p>
	</div>
{/snippet}
<Confirm
	msg="Are you sure you want to delete this server?"
	show={deleteServer}
	onsuccess={async () => {
		if (!catalogId || !entry) return;
		if (type === 'single' || type === 'remote') {
			await AdminService.deleteMCPCatalogEntry(catalogId, entry.id);
		} else {
			await AdminService.deleteMCPCatalogServer(catalogId, entry.id);
		}
		goto('/v2/admin/mcp-servers');
	}}
	oncancel={() => (deleteServer = false)}
/>

<Confirm
	msg={deleteResourceFromRule?.resourceId === '*'
		? 'Are you sure you want to remove Everything from this rule?'
		: 'Are you sure you want to remove this MCP server from this rule?'}
	show={Boolean(deleteResourceFromRule)}
	onsuccess={async () => {
		if (!deleteResourceFromRule) {
			return;
		}
		await AdminService.updateAccessControlRule(deleteResourceFromRule.rule.id, {
			...deleteResourceFromRule.rule,
			resources: deleteResourceFromRule.rule.resources?.filter(
				(r) => r.id !== deleteResourceFromRule!.resourceId
			)
		});

		listAccessControlRules = AdminService.listAccessControlRules();
		deleteResourceFromRule = undefined;
	}}
	oncancel={() => (deleteResourceFromRule = undefined)}
/>
