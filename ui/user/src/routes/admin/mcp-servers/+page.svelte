<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import McpServerGitSync from '$lib/components/admin/McpServerGitSync.svelte';
	import ConnectorsView from '$lib/components/mcp/ConnectorsView.svelte';
	import DeploymentsView from '$lib/components/mcp/DeploymentsView.svelte';
	import SelectServerType from '$lib/components/mcp/SelectServerType.svelte';
	import type { InitSort } from '$lib/components/table/Table.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, Group, type LaunchServerType } from '$lib/services';
	import type { MCPCatalog, OrgUser } from '$lib/services/admin/types';
	import { getServerTypeLabelByType } from '$lib/services/chat/mcp';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import { goto } from '$lib/url';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setSortUrlParams,
		setFilterUrlParams,
		setUrlParam,
		replaceState
	} from '$lib/url';
	import SourceUrlsView from './SourceUrlsView.svelte';
	import { Info, LoaderCircle, Plus, RefreshCcw, Server } from 'lucide-svelte';
	import { onDestroy, onMount } from 'svelte';
	import { fade, fly, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type View = 'registry' | 'deployments' | 'urls';

	let view = $state<View>((page.url.searchParams.get('view') as View) || 'registry');
	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;

	const { data } = $props();
	const { workspaceId } = $derived(data);
	const query = $derived(page.url.searchParams.get('query') || '');

	let users = $state<OrgUser[]>([]);
	let urlFilters = $state(getTableUrlParamsFilters());
	let initSort = $derived.by(() => {
		const defValue =
			view === 'deployments'
				? ({
						property: 'created',
						order: 'desc'
					} as InitSort)
				: undefined;

		return getTableUrlParamsSort(defValue);
	});

	onMount(async () => {
		users = await AdminService.listUsersIncludeDeleted();
		defaultCatalog = await AdminService.getMCPCatalog(defaultCatalogId);

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

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let canCreateServer = $derived(
		profile.current.groups.includes(Group.ADMIN) || profile.current.groups.includes(Group.POWERUSER)
	);

	function selectServerType(type: LaunchServerType, updateUrl = true) {
		selectServerTypeDialog?.close();
		if (updateUrl) {
			goto(resolve(`/admin/mcp-servers?new=${type}`));
		}
	}

	function pollTillSyncComplete() {
		if (syncInterval) {
			clearInterval(syncInterval);
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
		const newUrl = new URL(page.url);
		setUrlParam(newUrl, 'query', value || null);
		navigateWithState(newUrl);
	};
</script>

<Layout
	classes={{ navbar: 'bg-surface1' }}
	title={showServerForm
		? `Create ${getServerTypeLabelByType(selectedServerType)} Server`
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
		{#if !isAdminReadonly && !showServerForm}
			<button class="button flex items-center gap-1 text-sm" onclick={sync}>
				{#if syncing}
					<LoaderCircle class="size-4 animate-spin" /> Syncing...
				{:else}
					<RefreshCcw class="size-4" />
					Sync
				{/if}
			</button>
		{/if}
		{#if canCreateServer}
			{@render addServerButton()}
		{/if}
	{/snippet}
</Layout>

{#snippet mainContent()}
	<div
		class="flex min-h-full flex-col"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div class="bg-surface1 dark:bg-background sticky top-16 left-0 z-20 w-full py-1">
			<div class="mb-2">
				<Search
					class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
					value={query}
					onChange={updateSearchQuery}
					placeholder={view !== 'urls' ? 'Search servers...' : 'Search sources...'}
				/>
			</div>
		</div>
		<div class="dark:bg-surface2 bg-background rounded-t-md shadow-sm">
			<div class="flex">
				<button
					class={twMerge('page-tab', view === 'registry' && 'page-tab-active')}
					onclick={() => switchView('registry')}
				>
					Server Entries
				</button>
				<button
					class={twMerge('page-tab', view === 'deployments' && 'page-tab-active')}
					onclick={() => switchView('deployments')}
				>
					Deployments & Connections
				</button>
				<button
					class={twMerge('page-tab', view === 'urls' && 'page-tab-active')}
					onclick={() => switchView('urls')}
				>
					Registry Sources
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

			{#if view === 'registry'}
				<ConnectorsView
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
				</ConnectorsView>
			{:else if view === 'urls'}
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
			{:else if view === 'deployments'}
				<DeploymentsView
					id={defaultCatalogId}
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
				</DeploymentsView>
			{/if}
		</div>
	</div>
{/snippet}

{#snippet displayNoData()}
	<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
		<Server class="text-on-surface1 size-24 opacity-25" />
		<h4 class="text-on-surface1 text-lg font-semibold">No created MCP servers</h4>
		<p class="text-on-surface1 text-sm font-light">
			Looks like you don't have any servers created yet. <br />
			Click the button below to get started.
		</p>

		{#if canCreateServer}
			{@render addServerButton()}
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
				goto('/admin/mcp-servers');
			}}
			onSubmit={async (id, type, message) => {
				setUrlParam(page.url, 'new', null);
				replaceState(page.url, {});

				let queryParam = '?launch=true';
				if (message === 'requires-oauth-config') {
					queryParam = '?configure-oauth=true';
				}

				if (profile.current.isAdmin?.()) {
					if (type === 'single' || type === 'remote' || type === 'composite') {
						goto(resolve(`/admin/mcp-servers/c/${id}${queryParam}`));
					} else {
						goto(resolve(`/admin/mcp-servers/s/${id}${queryParam}`));
					}
				} else {
					if (type === 'single' || type === 'remote' || type === 'composite') {
						goto(resolve(`/admin/mcp-servers/w/${workspaceId}/c/${id}${queryParam}`));
					} else {
						goto(resolve(`/admin/mcp-servers/w/${workspaceId}/s/${id}${queryParam}`));
					}
				}
			}}
		/>
	</div>
{/snippet}

{#snippet addServerButton()}
	<DotDotDot
		class="button-primary w-full text-sm md:w-fit"
		placement="bottom"
		classes={{ popover: 'z-50' }}
	>
		{#snippet icon()}
			<span class="flex items-center justify-center gap-1">
				<Plus class="size-4" /> Add MCP Server
			</span>
		{/snippet}
		<button
			class="menu-button"
			onclick={() => {
				selectServerTypeDialog?.open();
			}}
		>
			Add server
		</button>
		<button
			class="menu-button"
			onclick={() => {
				sourceDialog?.open();
			}}
		>
			Add server(s) from Git
		</button>
	</DotDotDot>
{/snippet}

<McpServerGitSync bind:this={sourceDialog} {defaultCatalog} onSync={sync} />
<SelectServerType bind:this={selectServerTypeDialog} onSelectServerType={selectServerType} />

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
