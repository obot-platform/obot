<script lang="ts">
	import { page } from '$app/state';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import TabLayout from '$lib/components/TabLayout.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpServerGitSync from '$lib/components/admin/McpServerGitSync.svelte';
	import MessagePoliciesView from '$lib/components/admin/MessagePoliciesView.svelte';
	import SelectServerType from '$lib/components/mcp/SelectServerType.svelte';
	import {
		DEFAULT_MCP_CATALOG_ID,
		MCP_ACCESS_POLICY_FIELD_IDS,
		MCP_FILTERS_FIELD_IDS
	} from '$lib/constants';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		Group,
		UserService,
		type LaunchServerType,
		type MCPCatalog,
		type OrgUser
	} from '$lib/services';
	import { mcpServersAndEntries, profile, version } from '$lib/stores';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		goto,
		setFilterUrlParams,
		setSortUrlParams
	} from '$lib/url';
	import DeploymentsView from './DeploymentsView.svelte';
	import EntriesView from './EntriesView.svelte';
	import FiltersView from './FiltersView.svelte';
	import McpPoliciesView from './McpPoliciesView.svelte';
	import SourceUrlsView from './SourceUrlsView.svelte';
	import TunnelsView from './TunnelsView.svelte';
	import { getCreatedEntryUrl } from './utils';
	import { Plus, RefreshCcw, Server } from '@lucide/svelte';
	import { onDestroy, onMount } from 'svelte';

	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;
	const viewValues = [
		'servers',
		'entries',
		'sources',
		'git-credentials',
		'deployments',
		'filters',
		'tunnels',
		'ai-judge-policies',
		'access-policies'
	] as const;
	const serverTypes: LaunchServerType[] = ['hosted', 'multi', 'remote'];

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
	let gitCredentials = $derived(data.gitCredentials);
	let filtersLoading = $state(false);
	let syncing = $state(false);
	let syncInterval = $state<ReturnType<typeof setInterval>>();

	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
	let messagePoliciesEnabled = $derived(version.current.messagePoliciesEnabled === true);
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
			const adminCreateViews = [
				'access-policies',
				'filters',
				'tunnels',
				...(messagePoliciesEnabled ? ['ai-judge-policies'] : [])
			];
			return isNewEntry || adminCreateViews.includes(selectedView);
		}
		return false;
	});
	let layoutTitle = $derived.by(() => {
		if (!creating) return m.nav_mcp_servers();
		switch (selectedView) {
			case 'entries':
				return m.mcps_add_mcp_server();
			case 'filters':
				return m.mcps_filters_create_filter();
			case 'tunnels':
				return m.mcps_tunnels_create_mcp_tunnel();
			case 'ai-judge-policies':
				return m.mcps_create_ai_judge_policy();
			case 'access-policies':
				return m.mcps_create_mcp_access_policy();
			default:
				return m.nav_mcp_servers();
		}
	});
	let views = $derived([
		...(hasAdminAccess || isPowerUser
			? [
					{
						label: m.mcps_servers_tab(),
						value: 'servers',
						content: servers,
						tooltip: m.mcps_servers_tab_tooltip()
					}
				]
			: []),
		...(hasAdminAccess
			? [
					{
						label: m.mcps_sources_tab(),
						value: 'sources',
						content: sources,
						tooltip: m.mcps_sources_tab_tooltip()
					},
					{
						label: m.mcps_deployments_tab(),
						value: 'deployments',
						content: deployments,
						tooltip: m.mcps_deployments_tab_tooltip()
					},
					{
						label: m.core_filters_title(),
						value: 'filters',
						content: filters,
						tooltip: m.mcps_filters_tab_tooltip()
					},
					{
						label: m.mcps_tunnels_tab(),
						value: 'tunnels',
						content: tunnels,
						tooltip: m.mcps_tunnels_tab_tooltip()
					}
				]
			: []),
		...(isPowerUserPlus || hasAdminAccess
			? [
					{
						label: m.mcps_access_policies_tab(),
						value: 'access-policies',
						content: accessPolicy,
						tooltip: m.mcps_access_policies_tab_tooltip()
					}
				]
			: []),
		...(hasAdminAccess && messagePoliciesEnabled
			? [
					{
						label: m.mcps_ai_judge_policies_tab(),
						value: 'ai-judge-policies',
						content: messagePolicies,
						tooltip: m.mcps_ai_judge_policies_tab_tooltip()
					}
				]
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

	function handleEntryCreated(id: string, _isMultiUserEntry: boolean, message?: string) {
		goto(getCreatedEntryUrl(id, newServerType, message), { replaceState: true });
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
				onSubmit={handleEntryCreated}
				excludeViews={['overview']}
			/>
		{:else if selectedView === 'filters'}
			{@render filters()}
		{:else if selectedView === 'tunnels'}
			{@render tunnels()}
		{:else if selectedView === 'ai-judge-policies'}
			{@render messagePolicies()}
		{:else if selectedView === 'access-policies'}
			{@render accessPolicy()}
		{/if}
	</Layout>
{:else}
	<TabLayout
		title={m.nav_mcp_servers()}
		defaultView="servers"
		rightNavActions={navActions}
		{views}
		classes={{ childrenContainer: 'max-w-none' }}
	/>
{/if}

{#snippet navActions(view: string)}
	{#if view === 'servers' && canCreateEntry && !isAdminReadonly}
		<button
			class="btn btn-primary btn-block w-full text-sm md:w-52"
			id="add-catalog-entry-button"
			onclick={() => selectServerTypeDialog?.open()}
		>
			<Plus class="size-4" />
			{m.mcps_add_mcp_server()}
		</button>
	{:else if view === 'sources' && hasAdminAccess && !isAdminReadonly}
		<button class="btn btn-secondary flex items-center gap-1 text-sm" onclick={sync}>
			{#if syncing}
				<Loading class="size-4" /> {m.mcps_syncing()}
			{:else}
				<RefreshCcw class="size-4" />
				{m.mcps_sync()}
			{/if}
		</button>
		<button
			id="add-catalog-source-button"
			class="btn btn-primary btn-block w-full text-sm md:w-52"
			onclick={() => sourceDialog?.open()}
		>
			<Plus class="size-4" />
			{m.mcps_add_source_url()}
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
			ariaLabel={m.mcps_add_new_filter()}
		>
			{#snippet icon()}
				<span class="flex items-center justify-center gap-1">
					<Plus class="size-4" />
					{m.mcps_add_new_filter()}
				</span>
			{/snippet}
			<button
				id={MCP_FILTERS_FIELD_IDS.createCustomBtn}
				class="menu-button"
				onclick={() => openCreate('filters')}
			>
				{m.mcps_create_custom()}
			</button>
			<button
				id={MCP_FILTERS_FIELD_IDS.createBuiltInBtn}
				class="menu-button"
				disabled={data.systemCatalogEntries.length === 0}
				onclick={() => filtersTab?.openBuiltInPicker()}
			>
				{m.mcps_create_from_built_in()}
			</button>
		</DotDotDot>
	{:else if view === 'tunnels' && !isAdminReadonly}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => openCreate('tunnels')}
		>
			<Plus class="size-4" />
			{m.mcps_tunnels_create_mcp_tunnel()}
		</button>
	{:else if view === 'ai-judge-policies' && messagePoliciesEnabled && !isAdminReadonly}
		<button
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => openCreate('ai-judge-policies')}
		>
			<Plus class="size-4" />
			{m.mcps_add_ai_judge_policy()}
		</button>
	{:else if view === 'access-policies' && !isAdminReadonly}
		<button
			id={MCP_ACCESS_POLICY_FIELD_IDS.addPolicyBtn}
			class="btn btn-primary flex items-center gap-1 text-sm"
			onclick={() => openCreate('access-policies')}
		>
			<Plus class="size-4" />
			{m.mcps_access_policies_add_access_policy()}
		</button>
	{/if}
{/snippet}

{#snippet servers()}
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

{#snippet messagePolicies()}
	<MessagePoliciesView
		messagePolicies={data.messagePolicies ?? []}
		policyDirection="tool-calls"
		creating={creating && selectedView === 'ai-judge-policies'}
	/>
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
		<h4 class="text-muted-content text-lg font-semibold">{m.mcps_no_created_servers()}</h4>
		<p class="text-muted-content text-sm font-light">
			{m.mcps_no_created_servers_description()}
		</p>
	</div>
{/snippet}

<McpServerGitSync bind:this={sourceDialog} {defaultCatalog} {gitCredentials} onSync={sync} />
<SelectServerType bind:this={selectServerTypeDialog} onSelectServerType={selectServerType} />
