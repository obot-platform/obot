<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		fetchMcpServerAndEntries,
		getAdminMcpServerAndEntries,
		initMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte';
	import { AdminService, type MCPCatalogServer } from '$lib/services';
	import type { MCPCatalog, MCPCatalogEntry, OrgUser } from '$lib/services/admin/types';
	import { AlertTriangle, Info, LoaderCircle, Plus, RefreshCcw, Server, X } from 'lucide-svelte';
	import { onDestroy, onMount } from 'svelte';
	import { fade, fly, slide } from 'svelte/transition';
	import { beforeNavigate, goto } from '$app/navigation';
	import { afterNavigate } from '$app/navigation';
	import { browser } from '$app/environment';
	import SelectServerType, {
		type SelectServerOption
	} from '$lib/components/mcp/SelectServerType.svelte';
	import { profile } from '$lib/stores';
	import { page } from '$app/state';
	import DeploymentsView from '$lib/components/mcp/DeploymentsView.svelte';
	import Search from '$lib/components/Search.svelte';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setSortUrlParams,
		setFilterUrlParams
	} from '$lib/url';
	import { getServerTypeLabelByType } from '$lib/services/chat/mcp';
	import { debounce } from 'es-toolkit';
	import { localState } from '$lib/runes/localState.svelte';
	import RegistriesView from '$lib/components/mcp/RegistriesView.svelte';
	import SourceUrlsView from './SourceUrlsView.svelte';
	import { twMerge } from 'tailwind-merge';
	import DotDotDot from '$lib/components/DotDotDot.svelte';

	type View = 'registry' | 'deployments' | 'urls';

	let view = $state<View>((page.url.searchParams.get('view') as View) || 'registry');
	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;

	const query = $derived(page.url.searchParams.get('query') || '');

	type LocalStorageViewQuery = Record<View, string>;
	const localStorageViewQuery = localState<LocalStorageViewQuery>(
		'@obot/admin/mcp-servers/search-query',
		{ registry: '', deployments: '', urls: '' }
	);
	initMcpServerAndEntries();

	const mcpServerAndEntries = getAdminMcpServerAndEntries();
	let users = $state<OrgUser[]>([]);
	let urlFilters = $state(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

	onMount(async () => {
		users = await AdminService.listUsersIncludeDeleted();

		await fetchMcpServerAndEntries(defaultCatalogId, mcpServerAndEntries);
		defaultCatalog = await AdminService.getMCPCatalog(defaultCatalogId);

		if (defaultCatalog?.isSyncing) {
			pollTillSyncComplete();
		}
	});

	function handleFilter(property: string, values: string[]) {
		urlFilters[property] = values;
		setFilterUrlParams(property, values);
	}

	function handleClearAllFilters() {
		urlFilters = {};
		clearUrlParams();
	}

	afterNavigate(({ to }) => {
		if (browser && to?.url) {
			const serverId = to.url.searchParams.get('id');
			const createNewType = to.url.searchParams.get('new') as
				| 'single'
				| 'multi'
				| 'remote'
				| 'composite';
			if (createNewType) {
				selectServerType(createNewType, false);
			} else if (!serverId && (selectedEntryServer || showServerForm)) {
				selectedEntryServer = undefined;
				showServerForm = false;
			}
		}
	});

	beforeNavigate(({ to }) => {
		if (browser && !to?.url.pathname.startsWith('/admin/mcp-servers')) {
			clearQueryFromLocalStorage();
		}
	});

	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));

	let defaultCatalog = $state<MCPCatalog>();
	let editingSource = $state<{ index: number; value: string }>();
	let sourceDialog = $state<HTMLDialogElement>();
	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerType>>();
	let selectedServerType = $state<SelectServerOption>();
	let selectedEntryServer = $state<MCPCatalogEntry | MCPCatalogServer>();

	let showServerForm = $state(false);
	let saving = $state(false);
	let syncing = $state(false);
	let sourceError = $state<string>();
	let syncInterval = $state<ReturnType<typeof setInterval>>();

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	function selectServerType(type: SelectServerOption, updateUrl = true) {
		selectedServerType = type;
		selectServerTypeDialog?.close();
		showServerForm = true;
		if (updateUrl) {
			goto(`/admin/mcp-servers?new=${type}`, { replaceState: false });
		}
	}

	function closeSourceDialog() {
		editingSource = undefined;
		sourceError = undefined;
		sourceDialog?.close();
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
				fetchMcpServerAndEntries(defaultCatalogId, mcpServerAndEntries);
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

	// Helper function to update URL search params
	function setUrlParam(url: URL, key: string, value: string | null): void {
		if (value) {
			url.searchParams.set(key, value);
		} else {
			url.searchParams.delete(key);
		}
	}

	// Helper function to persist query to local storage
	function persistQueryToLocalStorage(view: View, queryValue: string): void {
		if (!localStorageViewQuery.current) {
			// Do nothing if local value has not loaded yet
			return;
		}

		localStorageViewQuery.current[view] = queryValue;
	}

	function clearQueryFromLocalStorage(view?: View): void {
		if (!localStorageViewQuery.current) {
			// Do nothing if local value has not loaded yet
			return;
		}

		if (view) {
			localStorageViewQuery.current[view] = '';
		} else {
			localStorageViewQuery.current = { registry: '', deployments: '', urls: '' };
		}
	}

	// Helper function to navigate with consistent options
	function navigateWithState(url: URL): void {
		goto(url.toString(), { replaceState: true, noScroll: true, keepFocus: true });
	}

	async function switchView(newView: View) {
		clearUrlParams(Array.from(page.url.searchParams.keys()).filter((key) => key !== 'query'));
		view = newView;

		const savedQuery = localStorageViewQuery.current?.[newView] || '';

		const newUrl = new URL(page.url);
		setUrlParam(newUrl, 'view', newView);
		setUrlParam(newUrl, 'query', savedQuery || null);

		navigateWithState(newUrl);
	}

	onDestroy(() => {
		if (syncInterval) {
			clearInterval(syncInterval);
		}
	});

	const duration = PAGE_TRANSITION_DURATION;

	const updateSearchQuery = debounce((value: string) => {
		const newUrl = new URL(page.url);

		setUrlParam(newUrl, 'query', value || null);

		persistQueryToLocalStorage(view, value);
		navigateWithState(newUrl);
	}, 100);
</script>

<Layout
	classes={{ navbar: 'bg-surface1' }}
	title={showServerForm
		? `Create ${getServerTypeLabelByType(selectedServerType)} Server`
		: 'MCP Servers'}
	showBackButton={showServerForm}
>
	<div class="flex min-h-full flex-col gap-8 pt-4" in:fade>
		{#if showServerForm}
			{@render configureEntryScreen()}
		{:else}
			{@render mainContent()}
		{/if}
	</div>
	{#snippet rightNavActions()}
		{#if !isAdminReadonly}
			<button class="button flex items-center gap-1 text-sm" onclick={sync}>
				{#if syncing}
					<LoaderCircle class="size-4 animate-spin" /> Syncing...
				{:else}
					<RefreshCcw class="size-4" />
					Sync
				{/if}
			</button>
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
		<div class="bg-surface1 dark:bg-background sticky top-16 left-0 z-20 w-full pb-1">
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
					Registry Entries
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
				<RegistriesView
					bind:catalog={defaultCatalog}
					readonly={isAdminReadonly}
					{usersMap}
					query={localStorageViewQuery.current?.['registry'] || ''}
					{urlFilters}
					onFilter={handleFilter}
					onClearAllFilters={handleClearAllFilters}
					onSort={setSortUrlParams}
					{initSort}
					classes={{
						tableHeader: 'top-31'
					}}
				>
					{#snippet noDataContent()}{@render displayNoData()}{/snippet}
				</RegistriesView>
			{:else if view === 'urls'}
				<SourceUrlsView
					catalog={defaultCatalog}
					readonly={isAdminReadonly}
					query={localStorageViewQuery.current?.['urls'] || ''}
					{syncing}
					onSync={sync}
				/>
			{:else if view === 'deployments'}
				<DeploymentsView
					catalogId={defaultCatalogId}
					readonly={isAdminReadonly}
					{usersMap}
					query={localStorageViewQuery.current?.['deployments'] || ''}
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

		{@render addServerButton()}
	</div>
{/snippet}

{#snippet configureEntryScreen()}
	<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		<McpServerEntryForm
			type={selectedServerType}
			id={defaultCatalogId}
			onCancel={() => {
				selectedEntryServer = undefined;
				showServerForm = false;
			}}
			onSubmit={async (id, type) => {
				if (type === 'single' || type === 'remote' || type === 'composite') {
					goto(`/admin/mcp-servers/c/${id}`);
				} else {
					goto(`/admin/mcp-servers/s/${id}`);
				}
			}}
		/>
	</div>
{/snippet}

{#snippet addServerButton()}
	<DotDotDot class="button-primary w-full text-sm md:w-fit" placement="bottom">
		{#snippet icon()}
			<span class="flex items-center justify-center gap-1">
				<Plus class="size-4" /> Add MCP Server
			</span>
		{/snippet}
		<div class="default-dialog flex min-w-max flex-col p-2">
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
					editingSource = {
						index: -1,
						value: ''
					};
					sourceDialog?.showModal();
				}}
			>
				Add server(s) from Git
			</button>
		</div>
	</DotDotDot>
{/snippet}

<dialog
	bind:this={sourceDialog}
	use:clickOutside={() => closeSourceDialog()}
	class="w-full max-w-md p-4"
>
	{#if editingSource}
		<h3 class="default-dialog-title">
			{editingSource.index === -1 ? 'Add Source URL' : 'Edit Source URL'}
			<button onclick={() => closeSourceDialog()} class="icon-button">
				<X class="size-5" />
			</button>
		</h3>

		<div class="my-4 flex flex-col gap-1">
			<label for="catalog-source-name" class="flex-1 text-sm font-light capitalize"
				>Source URL
			</label>
			<input id="catalog-source-name" bind:value={editingSource.value} class="text-input-filled" />
		</div>

		{#if sourceError}
			<div class="mb-4 flex flex-col gap-2 text-red-500 dark:text-red-400">
				<div class="flex items-center gap-2">
					<AlertTriangle class="size-6 flex-shrink-0 self-start" />
					<p class="my-0.5 flex flex-col text-sm font-semibold">Error adding source URL:</p>
				</div>
				<span class="font-sm font-light break-all">{sourceError}</span>
			</div>
		{/if}

		<div class="flex w-full justify-end gap-2">
			<button class="button" disabled={saving} onclick={() => closeSourceDialog()}>Cancel</button>
			<button
				class="button-primary"
				disabled={saving}
				onclick={async () => {
					if (!editingSource || !defaultCatalog) {
						return;
					}

					saving = true;
					sourceError = undefined;

					try {
						const updatingCatalog = { ...defaultCatalog };

						if (editingSource.index === -1) {
							updatingCatalog.sourceURLs = [
								...(updatingCatalog.sourceURLs ?? []),
								editingSource.value
							];
						} else {
							updatingCatalog.sourceURLs[editingSource.index] = editingSource.value;
						}

						const response = await AdminService.updateMCPCatalog(
							defaultCatalogId,
							updatingCatalog,
							{
								dontLogErrors: true
							}
						);
						defaultCatalog = response;
						await sync();
						closeSourceDialog();
					} catch (error) {
						sourceError = error instanceof Error ? error.message : 'An unexpected error occurred';
					} finally {
						saving = false;
					}
				}}
			>
				Add
			</button>
		</div>
	{/if}
</dialog>

<SelectServerType bind:this={selectServerTypeDialog} onSelectServerType={selectServerType} />

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
