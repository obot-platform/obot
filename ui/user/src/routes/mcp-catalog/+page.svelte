<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpServerGitSync from '$lib/components/admin/McpServerGitSync.svelte';
	import SelectServerType from '$lib/components/mcp/SelectServerType.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		Group,
		UserService,
		type LaunchServerType,
		type MCPCatalog,
		type OrgUser
	} from '$lib/services';
	import { getServerTypeLabelByType } from '$lib/services/user/mcp';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import {
		goto,
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setSortUrlParams,
		setFilterUrlParams,
		setUrlParam,
		replaceState,
		setUrlParamAndUpdateUrl
	} from '$lib/url';
	import EntriesView from './EntriesView.svelte';
	import SourceUrlsView from './SourceUrlsView.svelte';
	import { Info, Plus, RefreshCcw, Server, TriangleAlert } from 'lucide-svelte';
	import { onDestroy, onMount } from 'svelte';
	import { fade, fly, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type View = 'entries' | 'urls';

	let view = $state<View>((page.url.searchParams.get('view') as View) || 'entries');
	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;

	const { data } = $props();
	const { workspaceId } = $derived(data);
	const query = $derived(page.url.searchParams.get('query') || '');

	let users = $state<OrgUser[]>([]);
	let urlFilters = $state(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

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

	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));

	let defaultCatalog = $state<MCPCatalog>();
	let sourceDialog = $state<ReturnType<typeof McpServerGitSync>>();
	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerType>>();

	let selectedServerType = $derived(page.url.searchParams.get('new') as LaunchServerType);
	let showServerForm = $derived(page.url.searchParams.has('new'));

	let syncing = $state(false);
	let syncInterval = $state<ReturnType<typeof setInterval>>();

	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let canCreateEntry = $derived(
		profile.current.groups.includes(Group.ADMIN) || profile.current.groups.includes(Group.POWERUSER)
	);
	let prefix = $derived(hasAdminAccess ? '/admin' : '');

	function selectServerType(type: LaunchServerType, updateUrl = true) {
		selectServerTypeDialog?.close();
		if (updateUrl) {
			goto(`${prefix}/mcp-catalog?new=${type}`);
		}
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

	// Helper function to navigate with consistent options
	function navigateWithState(url: URL): void {
		goto(url, { replaceState: true, noScroll: true, keepFocus: true });
	}

	async function switchView(newView: View) {
		clearUrlParams(Array.from(page.url.searchParams.keys()).filter((key) => key !== 'query'));
		view = newView;

		const newUrl = new URL(page.url);
		setUrlParam(newUrl, 'view', newView);
		setUrlParam(newUrl, 'query', null);

		urlFilters = getTableUrlParamsFilters();

		navigateWithState(newUrl);
	}

	onDestroy(() => {
		if (syncInterval) {
			clearInterval(syncInterval);
		}
	});

	const duration = PAGE_TRANSITION_DURATION;

	const updateSearchQuery = (value: string) => {
		urlFilters = getTableUrlParamsFilters();
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	};
</script>

<Layout
	classes={{ navbar: 'bg-base-200', container: 'pt-0' }}
	title={showServerForm
		? `Create ${getServerTypeLabelByType(selectedServerType)}${selectedServerType !== 'multi' ? ' Catalog Entry' : ''}`
		: 'MCP Servers'}
	showBackButton={showServerForm}
>
	<div class="flex min-h-full flex-col gap-8" in:fade>
		{#if showServerForm}
			{@render configureEntryScreen()}
		{:else}
			{@render mainContent()}
		{/if}
	</div>
	{#snippet rightNavActions()}
		{#if hasAdminAccess && !isAdminReadonly && !showServerForm && view === 'urls'}
			<button
				in:fade={{ duration }}
				class="btn btn-secondary flex items-center gap-1 text-sm"
				onclick={sync}
			>
				{#if syncing}
					<Loading class="size-4" /> Syncing...
				{:else}
					<RefreshCcw class="size-4" />
					Sync
				{/if}
			</button>
		{/if}
		{#if canCreateEntry}
			{@render addButton()}
		{/if}
	{/snippet}
</Layout>

{#snippet mainContent()}
	<div
		class="flex min-h-full flex-col gap-2"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !hasAdminAccess}
			<div class="sticky top-16 left-0 z-20 w-full py-0.5">
				<Search
					class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
					value={query}
					onChange={updateSearchQuery}
					placeholder={view !== 'urls' ? 'Search servers...' : 'Search sources...'}
				/>
			</div>
		{/if}
		<div class="dark:bg-base-300 bg-base-100 rounded-t-md shadow-sm">
			{#if hasAdminAccess}
				<div class="flex">
					<button
						class={twMerge('page-tab max-w-1/2', view === 'entries' && 'page-tab-active')}
						onclick={() => switchView('entries')}
					>
						Catalog Entries
					</button>
					<button
						class={twMerge('page-tab max-w-1/2', view === 'urls' && 'page-tab-active')}
						onclick={() => switchView('urls')}
					>
						Catalog Sources
					</button>
				</div>

				{#if defaultCatalog?.isSyncing}
					<div class="p-4" transition:slide={{ axis: 'y' }}>
						<div class="notification-info p-3 text-sm font-light">
							<div class="flex items-center gap-3">
								<Info class="size-6" />
								<div>The catalog is currently syncing with your configured Git repositories.</div>
							</div>
						</div>
					</div>
				{/if}
			{/if}

			{#if view === 'entries' && defaultCatalog && !defaultCatalog.isSyncing && Object.keys(defaultCatalog.syncErrors ?? {}).length > 0}
				<div class="w-full p-4" in:slide={{ axis: 'y' }} out:slide={{ axis: 'y', duration: 0 }}>
					<div class="notification-alert flex w-full items-center gap-2 rounded-md p-3 text-sm">
						<TriangleAlert class="size-" />
						<p class="">
							Some servers failed to sync. See "Registry Sources" tab for more details.
						</p>
					</div>
				</div>
			{/if}

			{#if hasAdminAccess}
				<div class="bg-base-100 dark:bg-base-300 sticky top-16 left-0 z-20 w-full">
					<Search
						class="dark:bg-base-300 dark:border-base-400 bg-base-100 border-0 border-t border-base-300 ring-inset"
						value={query}
						onChange={updateSearchQuery}
						placeholder={view !== 'urls' ? 'Search servers...' : 'Search sources...'}
					/>
				</div>
			{/if}

			{#if view === 'entries'}
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
					onConnect={({ instance }) => {
						if (instance) {
							mcpServersAndEntries.refreshUserInstances();
						} else {
							mcpServersAndEntries.refreshUserConfiguredServers();
						}
					}}
				>
					{#snippet noDataContent()}{@render displayNoData()}{/snippet}
				</EntriesView>
			{:else if view === 'urls' && hasAdminAccess}
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
			{/if}
		</div>
	</div>
{/snippet}

{#snippet displayNoData()}
	<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
		<Server class="text-muted-content size-24 opacity-25" />
		<h4 class="text-muted-content text-lg font-semibold">No created entries</h4>
		<p class="text-muted-content text-sm font-light">
			Looks like you don't have any entries created yet. <br />
			Click the button below to get started.
		</p>

		{#if canCreateEntry}
			{@render addButton()}
		{/if}
	</div>
{/snippet}

{#snippet configureEntryScreen()}
	<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		<McpServerEntryForm
			entity={profile.current.isAdmin?.() ? 'catalog' : 'workspace'}
			type={selectedServerType}
			id={profile.current.isAdmin?.() ? defaultCatalogId : (workspaceId ?? '')}
			onCancel={() => {
				goto('/admin/mcp-catalog');
			}}
			onSubmit={async (id, _isMultiUserEntry, message) => {
				setUrlParam(page.url, 'new', null);
				replaceState(page.url, {});

				let queryParam = '?launch=true';
				if (message === 'requires-oauth-config') {
					queryParam = '?configure-oauth=true';
				}

				if (selectedServerType !== 'multi') {
					goto(`${prefix}/mcp-catalog/c/${id}${queryParam}`);
				} else {
					goto(`${prefix}/mcp-catalog/s/${id}${queryParam}`);
				}
			}}
			excludeViews={['overview']}
		/>
	</div>
{/snippet}

{#snippet addButton()}
	{#if canCreateEntry && !showServerForm}
		<div in:fade={{ duration }}>
			{#if view === 'entries'}
				<button
					class="btn btn-primary btn-block w-full text-sm md:w-52"
					onclick={() => selectServerTypeDialog?.open()}
				>
					<Plus class="size-4" /> Add Catalog Entry
				</button>
			{:else if view === 'urls'}
				<button
					class="btn btn-primary btn-block w-full text-sm md:w-52"
					onclick={() => sourceDialog?.open()}
				>
					<Plus class="size-4" /> Add Catalog Source
				</button>
			{/if}
		</div>
	{/if}
{/snippet}

<McpServerGitSync bind:this={sourceDialog} {defaultCatalog} onSync={sync} />
<SelectServerType bind:this={selectServerTypeDialog} onSelectServerType={selectServerType} />

<svelte:head>
	<title>Obot | MCP Management | MCP Servers</title>
</svelte:head>
