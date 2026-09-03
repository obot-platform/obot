<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import TabLayout from '$lib/components/TabLayout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpServerGitSync from '$lib/components/admin/McpServerGitSync.svelte';
	import SelectServerType from '$lib/components/mcp/SelectServerType.svelte';
	import {
		DEFAULT_MCP_CATALOG_ID,
		MCP_ACCESS_POLICY_FIELD_IDS,
		MCP_FILTERS_FIELD_IDS
	} from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		Group,
		UserService,
		type LaunchServerType,
		type MCPCatalog,
		type OrgUser
	} from '$lib/services';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		goto,
		setFilterUrlParams,
		setSortUrlParams
	} from '$lib/url';
	import ConnectorsView from './ConnectorsView.svelte';
	import DeploymentsView from './DeploymentsView.svelte';
	import EntriesView from './EntriesView.svelte';
	import FiltersView from './FiltersView.svelte';
	import McpPoliciesView from './McpPoliciesView.svelte';
	import SourceUrlsView from './SourceUrlsView.svelte';
	import TunnelsView from './TunnelsView.svelte';
	import { Plus, RefreshCcw, Server, Settings } from '@lucide/svelte';
	import { onDestroy, onMount, untrack } from 'svelte';

	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;
	const viewValues = [
		'servers',
		'entries',
		'sources',
		'git-credentials',
		'deployments',
		'filters',
		'tunnels',
		'access-policies'
	] as const;
	const serverTypes: LaunchServerType[] = ['hosted', 'multi', 'remote', 'composite'];

	const { data } = $props();
	const { workspaceId } = $derived(data);
	const query = $derived(page.url.searchParams.get('query') || '');

	let users = $state<OrgUser[]>([]);
	let urlFilters = $state(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());
	let defaultCatalog = $state<MCPCatalog>();
	let sourceDialog = $state<ReturnType<typeof McpServerGitSync>>();
	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerType>>();
	let filtersTab = $state<ReturnType<typeof FiltersView>>();
	let gitCredentials = $state(untrack(() => data.gitCredentials));
	let filtersLoading = $state(false);
	let syncing = $state(false);
	let syncInterval = $state<ReturnType<typeof setInterval>>();

	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let isPowerUser = $derived(profile.current.groups.includes(Group.POWERUSER));
	let isPowerUserPlus = $derived(profile.current.groups.includes(Group.POWERUSER_PLUS));
	let canCreateEntry = $derived(
		profile.current.groups.includes(Group.ADMIN) || profile.current.groups.includes(Group.POWERUSER)
	);
	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));
	let selectedView = $derived.by(() => {
		const requested = page.url.searchParams.get('view');
		return requested && viewValues.includes(requested as (typeof viewValues)[number])
			? requested
			: 'servers';
	});
	// The servers view carries the chosen server type in `new`; every other view just flags `new`.
	let newServerType = $derived.by(() => {
		const requested = page.url.searchParams.get('new');
		return serverTypes.includes(requested as LaunchServerType)
			? (requested as LaunchServerType)
			: undefined;
	});
	let creating = $derived.by(() => {
		if (!page.url.searchParams.has('new')) return false;
		const isNewEntry = selectedView === 'entries' && !!newServerType;
		if (isPowerUser && isNewEntry) return true;
		if (isPowerUserPlus && (isNewEntry || selectedView === 'access-policies')) return true;
		if (hasAdminAccess && !isAdminReadonly) {
			return isNewEntry || ['access-policies', 'filters', 'tunnels'].includes(selectedView);
		}
		return false;
	});
	let layoutTitle = $derived.by(() => {
		if (!creating) return 'MCP Servers';
		switch (selectedView) {
			case 'entries':
				return 'Add Catalog Entry';
			case 'filters':
				return 'Create Filter';
			case 'tunnels':
				return 'Create MCP Tunnel';
			case 'access-policies':
				return 'Create MCP Access Policy';
			default:
				return 'MCP Servers';
		}
	});
	let views = $derived([
		{ label: 'Servers', value: 'servers', content: servers },
		...(hasAdminAccess || isPowerUser
			? [{ label: 'Entries', value: 'entries', content: entries }]
			: []),
		...(hasAdminAccess
			? [
					{ label: 'Sources', value: 'sources', content: sources },
					{ label: 'Filters', value: 'filters', content: filters },
					{ label: 'Tunnels', value: 'tunnels', content: tunnels },
					{ label: 'Deployments', value: 'deployments', content: deployments }
				]
			: []),
		...(isPowerUserPlus || hasAdminAccess
			? [{ label: 'Access Policies', value: 'access-policies', content: accessPolicy }]
			: [])
	]);

	onMount(async () => {
		users = await UserService.listUsersIncludeDeleted();
		defaultCatalog = profile.current.hasAdminAccess?.()
			? await AdminService.getMCPCatalog(defaultCatalogId)
			: undefined;

		if (defaultCatalog?.isSyncing) {
			pollTillSyncComplete();
		}
	});

	function handleFilter(property: string, values: string[]) {
		if (values.length === 0) {
			delete urlFilters[property];
			urlFilters = { ...urlFilters };
		} else {
			urlFilters[property] = values;
		}
		setFilterUrlParams(property, values);
	}

	function handleClearAllFilters() {
		urlFilters = {};
		clearUrlParams();
	}

	function pollTillSyncComplete() {
		if (syncInterval) {
			clearInterval(syncInterval);
		}

		if (!hasAdminAccess) {
			return;
		}

		syncInterval = setInterval(async () => {
			defaultCatalog = await AdminService.getMCPCatalog(defaultCatalogId);
			if (defaultCatalog && !defaultCatalog.isSyncing) {
				if (syncInterval) {
					clearInterval(syncInterval);
				}
				mcpServersAndEntries.refreshAll();
				syncing = false;
			}
		}, 5000);
	}

	async function sync() {
		if (!hasAdminAccess) {
			return;
		}

		syncing = true;
		await AdminService.refreshMCPCatalog(defaultCatalogId);
		defaultCatalog = await AdminService.getMCPCatalog(defaultCatalogId);
		if (defaultCatalog?.isSyncing) {
			pollTillSyncComplete();
		}
	}

	function selectServerType(type: LaunchServerType) {
		selectServerTypeDialog?.close();
		openCreate('entries', type);
	}

	function closeCreateScreen() {
		goto(`${page.url.pathname}?view=${selectedView}`);
	}

	function openCreate(view: string, value: string = 'true') {
		goto(`${page.url.pathname}?view=${view}&new=${value}`);
	}

	onDestroy(() => {
		if (syncInterval) {
			clearInterval(syncInterval);
		}
	});
</script>

<svelte:head>
	<title>Obot | {layoutTitle}</title>
</svelte:head>

{#if creating}
	<Layout title={layoutTitle} showBackButton onBackButtonClick={closeCreateScreen}>
		{#if selectedView === 'entries'}
			<McpServerEntryForm
				entity={hasAdminAccess ? 'catalog' : 'workspace'}
				id={hasAdminAccess ? defaultCatalogId : (workspaceId ?? '')}
				type={newServerType}
				onCancel={closeCreateScreen}
				excludeViews={['overview']}
			/>
		{:else if selectedView === 'filters'}
			{@render filters()}
		{:else if selectedView === 'tunnels'}
			{@render tunnels()}
		{:else if selectedView === 'access-policies'}
			{@render accessPolicy()}
		{/if}
	</Layout>
{:else}
	<TabLayout
		title="MCP Servers"
		defaultView="servers"
		rightNavActions={navActions}
		{views}
		classes={{ childrenContainer: 'max-w-none' }}
	/>
{/if}

{#snippet navActions(view: string)}
	{#if view === 'entries' && canCreateEntry && !isAdminReadonly}
		<button
			class="btn btn-primary btn-block w-full text-sm md:w-52"
			id="add-catalog-entry-button"
			onclick={() => selectServerTypeDialog?.open()}
		>
			<Plus class="size-4" /> Add Catalog Entry
		</button>
	{:else if view === 'sources' && hasAdminAccess && !isAdminReadonly}
		<button class="btn btn-secondary flex items-center gap-1 text-sm" onclick={sync}>
			{#if syncing}
				<Loading class="size-4" /> Syncing...
			{:else}
				<RefreshCcw class="size-4" />
				Sync
			{/if}
		</button>
		<a
			class="btn btn-secondary flex items-center gap-1 text-sm"
			href={resolve('/admin/platform?view=git-credentials')}
		>
			<Settings class="size-4" /> Manage Credentials
		</a>
		<button
			id="add-catalog-source-button"
			class="btn btn-primary btn-block w-full text-sm md:w-52"
			onclick={() => sourceDialog?.open()}
		>
			<Plus class="size-4" /> Add Catalog Source
		</button>
	{:else if view === 'filters' && !isAdminReadonly}
		{#if filtersLoading}
			<Loading class="size-4" />
		{/if}
		<DotDotDot
			class="btn btn-block btn-primary w-full text-sm md:w-fit"
			placement="bottom"
			classes={{ popover: 'z-50' }}
			id={MCP_FILTERS_FIELD_IDS.addFilterBtn}
			ariaLabel="Add New Filter"
		>
			{#snippet icon()}
				<span class="flex items-center justify-center gap-1">
					<Plus class="size-4" /> Add New Filter
				</span>
			{/snippet}
			<button
				id={MCP_FILTERS_FIELD_IDS.createCustomBtn}
				class="menu-button"
				onclick={() => openCreate('filters')}
			>
				Create Custom
			</button>
			<button
				id={MCP_FILTERS_FIELD_IDS.createBuiltInBtn}
				class="menu-button"
				disabled={data.systemCatalogEntries.length === 0}
				onclick={() => filtersTab?.openBuiltInPicker()}
			>
				Create From Built-in
			</button>
		</DotDotDot>
	{:else if view === 'tunnels' && !isAdminReadonly}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => openCreate('tunnels')}
		>
			<Plus class="size-4" />
			Create MCP Tunnel
		</button>
	{:else if view === 'access-policies' && !isAdminReadonly}
		<button
			id={MCP_ACCESS_POLICY_FIELD_IDS.addPolicyBtn}
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => openCreate('access-policies')}
		>
			<Plus class="size-4" /> Add Access Policy
		</button>
	{/if}
{/snippet}

{#snippet entries()}
	<EntriesView
		entity={profile.current.hasAdminAccess?.() ? 'catalog' : 'workspace'}
		id={profile.current.hasAdminAccess?.() ? defaultCatalogId : (workspaceId ?? '')}
		bind:catalog={defaultCatalog}
		readonly={isAdminReadonly}
		{usersMap}
		{query}
		{urlFilters}
		onFilter={handleFilter}
		onClearAllFilters={handleClearAllFilters}
		onSort={setSortUrlParams}
		{initSort}
	>
		{#snippet noDataContent()}{@render displayNoData()}{/snippet}
	</EntriesView>
{/snippet}

{#snippet servers()}
	<ConnectorsView {workspaceId} />
{/snippet}

{#snippet sources()}
	<SourceUrlsView
		catalog={defaultCatalog}
		readonly={isAdminReadonly}
		{query}
		{syncing}
		onSync={sync}
		onEdit={(url, index) => {
			sourceDialog?.edit(url, index);
		}}
	/>
{/snippet}

{#snippet deployments()}
	<DeploymentsView />
{/snippet}

{#snippet filters()}
	<FiltersView
		bind:this={filtersTab}
		bind:loading={filtersLoading}
		filters={data.filters}
		systemCatalogEntries={data.systemCatalogEntries}
	/>
{/snippet}

{#snippet tunnels()}
	<TunnelsView mcpTunnels={data.mcpTunnels} tunnelConnections={data.tunnelConnections} />
{/snippet}

{#snippet accessPolicy()}
	<McpPoliciesView
		accessControlRules={data.accessControlRules}
		creating={creating && selectedView === 'access-policies'}
		{workspaceId}
	/>
{/snippet}

{#snippet displayNoData()}
	<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
		<Server class="text-muted-content size-24 opacity-25" />
		<h4 class="text-muted-content text-lg font-semibold">No created entries</h4>
		<p class="text-muted-content text-sm font-light">
			Looks like you don't have any entries created yet.
		</p>
	</div>
{/snippet}

<McpServerGitSync bind:this={sourceDialog} {defaultCatalog} {gitCredentials} onSync={sync} />
<SelectServerType bind:this={selectServerTypeDialog} onSelectServerType={selectServerType} />
