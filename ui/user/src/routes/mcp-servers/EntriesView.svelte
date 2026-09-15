<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import McpDeprecatedNotice from '$lib/components/mcp/McpDeprecatedNotice.svelte';
	import McpDetachedNotice from '$lib/components/mcp/McpDetachedNotice.svelte';
	import McpTunnelDisconnectedStatus from '$lib/components/mcp/McpTunnelDisconnectedStatus.svelte';
	import Table, { type InitSort, type InitSortFn } from '$lib/components/table/Table.svelte';
	import { type MCPCatalog, type OrgUser } from '$lib/services';
	import { OBOT_PLATFORM_REPO } from '$lib/services/admin/constants';
	import {
		convertEntriesToTableData,
		isMultiUserCatalogEntry,
		hasEditableConfiguration,
		isDeprecatedMCPServer
	} from '$lib/services/user/mcp';
	import {
		getMcpTunnelConnectionsKey,
		isMcpTunnelDisconnected
	} from '$lib/services/user/mcpTunnel';
	import { mcpServersAndEntries, mcpTunnelConnections, profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import {
		CircleFadingArrowUp,
		GitBranch,
		Info,
		Server,
		Settings,
		TriangleAlert
	} from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { slide } from 'svelte/transition';

	type Item = ReturnType<typeof convertEntriesToTableData>[number];

	interface Props {
		entity?: 'workspace' | 'catalog';
		id?: string;
		catalog?: MCPCatalog;
		noDataContent?: Snippet;
		usersMap?: Map<string, OrgUser>;
		query?: string;
		urlFilters?: Record<string, (string | number)[]>;
		onFilter?: (property: string, values: string[]) => void;
		onClearAllFilters?: () => void;
		onSort?: InitSortFn;
		initSort?: InitSort;
		classes?: {
			tableHeader?: string;
		};
	}

	let {
		entity,
		id,
		catalog = $bindable(),
		noDataContent,
		urlFilters: filters,
		onFilter,
		onClearAllFilters,
		onSort,
		initSort = { property: 'name', order: 'asc' },
		classes,
		usersMap
	}: Props = $props();

	let query = $derived(page.url.searchParams.get('query') ?? '');

	let tableData = $derived(
		convertEntriesToTableData(
			mcpServersAndEntries.current.entries,
			usersMap,
			mcpServersAndEntries.current.userConfiguredServers,
			mcpServersAndEntries.current.servers
		).filter((d) => {
			const isOwnedByUser =
				profile.current.hasAdminAccess?.() ||
				(entity === 'workspace' && id && d.data.powerUserWorkspaceID === id);
			return isOwnedByUser;
		})
	);

	let filteredTableData = $derived.by(() => {
		const sorted = tableData.sort((a, b) => {
			return a.name.localeCompare(b.name);
		});
		return query
			? sorted.filter(
					(d) =>
						d.name.toLowerCase().includes(query.toLowerCase()) ||
						d.registry.toLowerCase().includes(query.toLowerCase())
				)
			: sorted;
	});
	let tunnelConnectionsKey = $derived(
		getMcpTunnelConnectionsKey(mcpTunnelConnections.current.connections)
	);

	let deploymentsNeedingAttentionByCatalogEntry = $derived(
		new Set<string>(
			mcpServersAndEntries.current.servers
				.filter((s) => s.catalogEntryID && (s.needsUpdate || s.needsK8sUpdate))
				?.map((s) => s.catalogEntryID)
		)
	);

	function getEntryUrl(d: Item, params: Record<string, string> = {}) {
		if (profile.current.hasAdminAccess?.() && d.data.powerUserWorkspaceID) {
			params = { ...params, wid: d.data.powerUserWorkspaceID };
		}
		const query = Object.entries(params)
			.map(([key, value]) => `${key}=${encodeURIComponent(value)}`)
			.join('&');
		return `/mcp-servers/c/${d.data.id}${query ? `?${query}` : ''}`;
	}

	const updateSearchQuery = (value: string) => {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	};
</script>

<div class="flex h-full w-full gap-2 flex-col">
	{#if catalog?.isSyncing}
		<div class="notification-info p-3 text-sm font-light" transition:slide={{ axis: 'y' }}>
			<div class="flex items-center gap-3">
				<Info class="size-6" />
				<div>The system is currently syncing with your configured Git repositories.</div>
			</div>
		</div>
	{/if}

	{#if mcpServersAndEntries.current.loading && tableData.length === 0}
		<Skeleton
			type="table"
			count={10}
			classes={{ header: 'h-14 rounded-none', body: 'rounded-none' }}
		/>
	{/if}
	{#if mcpServersAndEntries.current.isInitialized}
		{#if filteredTableData.length === 0}
			{#if noDataContent}
				<div class="flex flex-col gap-px">
					{@render noDataContent?.()}
				</div>
			{/if}
		{:else}
			<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
				<Search
					value={query}
					class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
					onChange={updateSearchQuery}
					placeholder="Search MCP servers..."
				/>
			</div>
			<Table
				data={filteredTableData}
				remeasureKey={tunnelConnectionsKey}
				fields={profile.current.hasAdminAccess?.()
					? ['name', 'type', 'users', 'created', 'source']
					: ['name', 'created']}
				filterable={['name', 'type', 'source']}
				{filters}
				onClickRow={(d, isCtrlClick) => {
					openUrl(getEntryUrl(d), isCtrlClick);
				}}
				{initSort}
				{onFilter}
				{onClearAllFilters}
				{onSort}
				sortable={['name', 'type', 'users', 'created', 'source']}
				noDataMessage="No catalog servers added."
				classes={{
					root: 'rounded-none rounded-b-md shadow-none',
					thead: classes?.tableHeader,
					row: 'min-h-14 py-3'
				}}
				setRowClasses={(d) => {
					const missingSecretBinding = 'missingKubernetesSecret' in d && d.missingKubernetesSecret;
					return (d.data.needsUpdate && !missingSecretBinding) ||
						deploymentsNeedingAttentionByCatalogEntry.has(d.data.id)
						? 'bg-primary/10'
						: '';
				}}
			>
				{#snippet onRenderColumn(property, d)}
					{@const attentionRequired =
						(d.data.needsUpdate &&
							!('missingKubernetesSecret' in d && d.missingKubernetesSecret)) ||
						deploymentsNeedingAttentionByCatalogEntry.has(d.data.id)}
					{@const deprecated = isDeprecatedMCPServer(d.data)}
					{@const tunnelDisconnected = isMcpTunnelDisconnected(
						d.data,
						mcpTunnelConnections.current.connections
					)}
					{#if property === 'name'}
						<div class="flex shrink-0 items-center gap-2">
							<div class="icon">
								{#if d.icon}
									<img src={d.icon} alt={d.name} class="size-6" />
								{:else}
									<Server class="size-6" />
								{/if}
							</div>
							<p class="flex items-center gap-2">
								{d.name}
								{#if tunnelDisconnected}
									<McpTunnelDisconnectedStatus />
								{/if}
								{#if attentionRequired}
									<span
										use:tooltip={{
											classes: ['border-primary', 'bg-primary/10', 'dark:bg-primary/50'],
											text: deploymentsNeedingAttentionByCatalogEntry.has(d.data.id)
												? 'One or multiple deployments require your attention'
												: 'Configuration requires your attention'
										}}
									>
										<CircleFadingArrowUp class="text-primary size-4" />
									</span>
								{:else if 'missingKubernetesSecret' in d && d.missingKubernetesSecret}
									<span
										class="text-warning"
										use:tooltip={{
											text:
												'missingKubernetesSecret' in d && d.missingKubernetesSecret
													? 'Missing Kubernetes Secret.'
													: 'Server requires an update.'
										}}
									>
										<TriangleAlert class="size-4" />
									</span>
								{/if}
								{#if d.status.toLowerCase() === 'deployed'}
									<span class="badge badge-xs badge-secondary">Deployed</span>
								{/if}
								{#if entity === 'catalog'}
									<McpDetachedNotice
										detached={d.data.detached}
										sourceURL={'sourceURL' in d.data ? d.data.sourceURL : undefined}
									/>
								{/if}
								<McpDeprecatedNotice {deprecated} />
							</p>
						</div>
					{:else if property === 'type'}
						{d.type}
						{#if !isMultiUserCatalogEntry(d.data) && hasEditableConfiguration(d.data)}
							<div class="p-2" use:tooltip={{ text: 'Requires user configuration' }}>
								<Settings class="size-3 text-muted-content" />
							</div>
						{/if}
					{:else if property === 'created'}
						{formatTimeAgo(d.created).relativeTime}
					{:else if property === 'source'}
						{#if d.sourceType === 'git'}
							<a
								onclick={(e) => e.stopPropagation()}
								href={d.source}
								target="_blank"
								rel="external noopener noreferrer"
								use:tooltip={{
									text: 'View Source on Git'
								}}
								class="link link-hover flex items-center gap-1 shrink-0 hover:text-blue-500"
							>
								<GitBranch class="size-4" />
								<span class="font-light">
									{#if d.source.startsWith(OBOT_PLATFORM_REPO)}
										Obot Catalog
									{:else}
										{d.source?.split('/').pop()}
									{/if}
								</span>
							</a>
						{:else}
							{d.source}
						{/if}
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
			</Table>
		{/if}
	{/if}
</div>
