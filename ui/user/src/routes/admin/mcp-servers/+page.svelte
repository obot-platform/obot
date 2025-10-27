<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		fetchMcpServerAndEntries,
		getAdminMcpServerAndEntries,
		initMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte';
	import { AdminService, type MCPCatalogServer } from '$lib/services';
	import type { MCPCatalog, MCPCatalogEntry, OrgUser } from '$lib/services/admin/types';
	import { AlertTriangle, Info, LoaderCircle, Plus, RefreshCcw, X } from 'lucide-svelte';
	import { onDestroy, onMount } from 'svelte';
	import { fade, fly, slide } from 'svelte/transition';
	import { goto, replaceState } from '$app/navigation';
	import { afterNavigate } from '$app/navigation';
	import { browser } from '$app/environment';
	import BackLink from '$lib/components/BackLink.svelte';
	import SelectServerType from '$lib/components/mcp/SelectServerType.svelte';
	import { profile } from '$lib/stores';
	import { page } from '$app/state';
	import { twMerge } from 'tailwind-merge';
	import RegistriesView from './RegistriesView.svelte';
	import DeploymentsView from './DeploymentsView.svelte';
	import Search from '$lib/components/Search.svelte';
	import SourceUrlsView from './SourceUrlsView.svelte';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setSortUrlParams,
		setFilterUrlParams
	} from '$lib/url';

	type View = 'registry' | 'deployments' | 'urls';

	let view = $state<View>((page.url.searchParams.get('view') as View) || 'registry');
	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;

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

	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));

	let defaultCatalog = $state<MCPCatalog>();
	let editingSource = $state<{ index: number; value: string }>();
	let sourceDialog = $state<HTMLDialogElement>();
	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerType>>();
	let selectedServerType = $state<'single' | 'multi' | 'remote' | 'composite'>();
	let selectedEntryServer = $state<MCPCatalogEntry | MCPCatalogServer>();
	let query = $state('');

	let showServerForm = $state(false);
	let saving = $state(false);
	let syncing = $state(false);
	let sourceError = $state<string>();
	let syncInterval = $state<ReturnType<typeof setInterval>>();

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let totalCount = $derived(
		mcpServerAndEntries.entries.length + mcpServerAndEntries.servers.length
	);

	function selectServerType(type: 'single' | 'multi' | 'remote' | 'composite', updateUrl = true) {
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

	async function switchView(newView: View) {
		clearUrlParams();
		view = newView;
		page.url.searchParams.set('view', newView);
		replaceState(page.url, {});
	}

	onDestroy(() => {
		if (syncInterval) {
			clearInterval(syncInterval);
		}
	});

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout classes={{ navbar: 'bg-surface1' }}>
	<div class="flex min-h-full flex-col gap-8 pt-4" in:fade>
		{#if showServerForm}
			{@render configureEntryScreen()}
		{:else}
			{@render mainContent()}
		{/if}
	</div>
</Layout>

{#snippet mainContent()}
	<div
		class="flex min-h-full flex-col"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div
			class="mb-4 flex flex-col items-center justify-start md:mb-8 md:flex-row md:justify-between"
		>
			<div class="flex items-center gap-2">
				<h1 class="text-2xl font-semibold">MCP Servers</h1>
				{#if !isAdminReadonly}
					<button class="button-small flex items-center gap-1 text-xs font-normal" onclick={sync}>
						{#if syncing}
							<LoaderCircle class="size-4 animate-spin" /> Syncing...
						{:else}
							<RefreshCcw class="size-4" />
							Sync
						{/if}
					</button>
				{/if}
			</div>
			{#if totalCount > 0 && !isAdminReadonly}
				<div class="mt-4 w-full flex-shrink-0 md:mt-0 md:w-fit">
					{@render addServerButton()}
				</div>
			{/if}
		</div>
		<div class="bg-surface1 sticky top-16 left-0 z-20 w-full pb-1 dark:bg-black">
			<div class="mb-2">
				<Search
					class="dark:bg-surface1 dark:border-surface3 border border-transparent bg-white shadow-sm"
					onChange={(val) => (query = val)}
					placeholder={view !== 'urls' ? 'Search servers...' : 'Search sources...'}
				/>
			</div>
		</div>
		<div class="dark:bg-surface2 rounded-t-md bg-white shadow-sm">
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
					{query}
					{urlFilters}
					onFilter={handleFilter}
					onClearAllFilters={handleClearAllFilters}
					onSort={setSortUrlParams}
					{initSort}
				>
					{#snippet emptyContentButton()}
						{@render addServerButton()}
					{/snippet}
				</RegistriesView>
			{:else if view === 'urls'}
				<SourceUrlsView
					catalog={defaultCatalog}
					readonly={isAdminReadonly}
					{query}
					{syncing}
					onSync={sync}
				/>
			{:else if view === 'deployments'}
				<DeploymentsView
					catalogId={defaultCatalogId}
					readonly={isAdminReadonly}
					{usersMap}
					{query}
					{urlFilters}
					onFilter={handleFilter}
					onClearAllFilters={handleClearAllFilters}
					onSort={setSortUrlParams}
					{initSort}
				/>
			{/if}
		</div>
	</div>
{/snippet}

{#snippet configureEntryScreen()}
	{@const currentLabelType =
		selectedServerType === 'single'
			? 'Single User'
			: selectedServerType === 'multi'
				? 'Multi-User'
				: selectedServerType === 'remote'
					? 'Remote'
					: 'Composite'}
	<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		<BackLink fromURL="mcp-servers" currentLabel={`Create ${currentLabelType} Server`} />
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
