<script lang="ts">
	import { AdminService, type MCPFilter, type MCPCatalogServer } from '$lib/services';
	import type { AccessControlRule, MCPCatalogEntry, OrgUser } from '$lib/services/admin/types';
	import { twMerge } from 'tailwind-merge';
	import McpServerInfo from '../mcp/McpServerInfo.svelte';
	import CatalogServerForm from './CatalogServerForm.svelte';
	import Table from '../Table.svelte';
	import { GlobeLock, ListFilter, LoaderCircle, Trash2, Users } from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { goto } from '$app/navigation';
	import Confirm from '../Confirm.svelte';
	import { ADMIN_SESSION_STORAGE } from '$lib/constants';
	import { onMount } from 'svelte';
	import AuditDetails from './audit-logs/AuditDetails.svelte';
	import McpServerInstances from './McpServerInstances.svelte';

	type MCPType = 'single' | 'multi' | 'remote';

	interface Props {
		catalogId?: string;
		entry?: MCPCatalogEntry | MCPCatalogServer;
		type?: MCPType;
		readonly?: boolean;
		onCancel?: () => void;
		onSubmit?: (id: string, type: MCPType) => void;
	}

	let { entry, catalogId, type, readonly, onCancel, onSubmit }: Props = $props();

	const tabs = $derived(
		entry
			? [
					{ label: 'Overview', view: 'overview' },
					{ label: 'Configuration', view: 'configuration' },
					{ label: 'Audit Logs', view: 'usage' },
					{ label: 'Access Control', view: 'access-control' },
					{ label: 'Server Instances', view: 'server-instances' },
					{ label: 'Filters', view: 'filters' }
				]
			: []
	);

	let listAccessControlRules = $state<Promise<AccessControlRule[]>>();
	let listEntryServers = $state<Promise<MCPCatalogServer[]>>();
	let listFilters = $state<Promise<MCPFilter[]>>();
	let users = $state<OrgUser[]>([]);

	let deleteServer = $state(false);
	let deleteResourceFromRule = $state<{
		rule: AccessControlRule;
		resourceId: string;
	}>();
	let view = $state<string>(entry ? 'overview' : 'configuration');

	$effect(() => {
		if (view === 'access-control') {
			listAccessControlRules = AdminService.listAccessControlRules();
		} else if (view === 'filters') {
			listFilters = AdminService.listMCPFilters();
		}
	});

	onMount(() => {
		AdminService.listUsers().then((data) => {
			users = data;
		});

		const url = new URL(window.location.href);
		const initialView = url.searchParams.get('view');
		if (initialView) {
			view = initialView;
		}
	});

	function filterRulesByEntry(rules?: AccessControlRule[]) {
		if (!entry || !rules) return [];
		return rules.filter((r) =>
			r.resources?.find((resource) => resource.id === entry.id || resource.id === '*')
		);
	}

	function filterFiltersByEntry(filters?: MCPFilter[]) {
		if (!entry || !filters) return [];
		return filters.filter((f) =>
			f.resources?.find((resource) => resource.id === entry.id || resource.id === '*')
		);
	}

	function setLastVisitedMcpServer() {
		if (!entry) return;
		const name =
			'manifest' in entry
				? entry.manifest?.name
				: (entry.commandManifest?.name ?? entry.urlManifest?.name);
		sessionStorage.setItem(
			ADMIN_SESSION_STORAGE.LAST_VISITED_MCP_SERVER,
			JSON.stringify({ id: entry.id, name, type })
		);
	}
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	class:mb-8={view !== 'configuration' &&
		view !== 'server-instances' &&
		view === 'configuration' &&
		readonly}
>
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
	<div class="flex grow flex-col gap-2">
		{#if tabs.length > 0}
			<div
				class="grid grid-cols-3 items-center gap-2 text-sm font-light md:grid-cols-4 lg:grid-cols-6"
			>
				{#each tabs as tab (tab.view)}
					<button
						onclick={() => {
							view = tab.view;
							const url = new URL(window.location.href);
							url.searchParams.set('view', tab.view);
							goto(url.toString(), { replaceState: true });
						}}
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
			<McpServerInfo
				{catalogId}
				{entry}
				descriptionPlaceholder="Add a description for this MCP server in the Configuration tab"
			/>
		{:else if view === 'configuration'}
			{@render configurationView()}
		{:else if view === 'access-control'}
			{@render accessControlView()}
		{:else if view === 'usage'}
			{@render usageView()}
		{:else if view === 'server-instances'}
			<McpServerInstances {catalogId} {entry} {users} {type} />
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
					setLastVisitedMcpServer();
					goto(
						`/v2/admin/access-control/${d.id}?from=${encodeURIComponent(`mcp-servers/${entry.id}`)}`
					);
				}}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'resources'}
						{@const referencedResource = d.resources?.find(
							(r) => r.id === entry?.id || r.id === '*'
						)}
						{referencedResource?.id === '*' ? 'Everything' : 'Self'}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
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
	{#if entry}
		{@const name = 'manifest' in entry ? entry.manifest.name : undefined}
		{@const mcpId = 'manifest' in entry ? entry.id : undefined}
		{@const mcpCatalogEntryId = !('manifest' in entry) ? entry.id : undefined}
		<AuditDetails
			mcpServerDisplayName={name}
			{mcpCatalogEntryId}
			{users}
			filters={{
				startTime: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString(),
				endTime: new Date().toISOString()
			}}
		>
			{#snippet emptyContent()}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Users class="size-24 text-gray-200 dark:text-gray-900" />
					<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
						No recent usage data
					</h4>
					<p class="text-sm font-light text-gray-400 dark:text-gray-600">
						This server has not had any active usage in the last 7 days.
					</p>
					{#if mcpId || mcpCatalogEntryId}
						{@const param = mcpId ? 'mcpId=' + mcpId : 'entryId=' + mcpCatalogEntryId}
						<p class="text-sm font-light text-gray-400 dark:text-gray-600">
							See more usage details in the server's <a
								href={`/v2/admin/audit-logs?${param}`}
								class="text-link"
							>
								Audit Logs
							</a>.
						</p>
					{/if}
				</div>
			{/snippet}
		</AuditDetails>
	{/if}
{/snippet}

{#snippet filtersView()}
	{#if listFilters}
		{#await listFilters}
			<div class="flex w-full justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:then filters}
			{@const serverFilters = entry ? filterFiltersByEntry(filters) : []}
			{#if serverFilters && serverFilters.length > 0}
				<Table
					data={serverFilters}
					fields={['name', 'url', 'selectors']}
					headers={[
						{ title: 'Name', property: 'name' },
						{ title: 'Webhook URL', property: 'url' },
						{ title: 'Selectors', property: 'selectors' }
					]}
					onSelectRow={(d) => {
						setLastVisitedMcpServer();
						goto(
							`/v2/admin/filters/${d.id}?from=${encodeURIComponent(`mcp-servers/${entry?.id}`)}`
						);
					}}
				>
					{#snippet onRenderColumn(property, d)}
						{#if property === 'name'}
							{d.name || '-'}
						{:else if property === 'url'}
							{d.url || '-'}
						{:else if property === 'selectors'}
							{@const count = d.selectors?.length || 0}
							{count > 0 ? `${count} selector${count > 1 ? 's' : ''}` : '-'}
						{:else}
							{d[property as keyof typeof d]}
						{/if}
					{/snippet}
				</Table>
			{:else}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<ListFilter class="size-24 text-gray-200 dark:text-gray-900" />
					<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
						No filters configured
					</h4>
					<p class="text-sm font-light text-gray-400 dark:text-gray-600">
						This server is not referenced by any filters.
					</p>
				</div>
			{/if}
		{/await}
	{:else}
		<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<ListFilter class="size-24 text-gray-200 dark:text-gray-900" />
			<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No filters available</h4>
			<p class="text-sm font-light text-gray-400 dark:text-gray-600">
				No filters have been configured in the system.
			</p>
		</div>
	{/if}
{/snippet}
<Confirm
	msg="Are you sure you want to delete this server?"
	show={deleteServer}
	onsuccess={async () => {
		if (!catalogId || !entry) return;
		if ('manifest' in entry) {
			await AdminService.deleteMCPCatalogServer(catalogId, entry.id);
		} else {
			await AdminService.deleteMCPCatalogEntry(catalogId, entry.id);
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
