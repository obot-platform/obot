<script lang="ts">
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { ChatService, Group, type MCPCatalogServer } from '$lib/services';
	import type { MCPCatalogEntry } from '$lib/services/admin/types';
	import { Plus, Server } from 'lucide-svelte';
	import { fade, fly } from 'svelte/transition';
	import { goto } from '$lib/url';
	import { beforeNavigate, afterNavigate, replaceState } from '$app/navigation';
	import { browser } from '$app/environment';
	import Search from '$lib/components/Search.svelte';
	import SelectServerType, {
		type SelectServerOption
	} from '$lib/components/mcp/SelectServerType.svelte';
	import { getServerTypeLabelByType } from '$lib/services/chat/mcp.js';
	import McpConfirmDelete from '$lib/components/mcp/McpConfirmDelete.svelte';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setFilterUrlParams,
		setSortUrlParams,
		setUrlParam
	} from '$lib/url';
	import { mcpServersAndEntries, profile } from '$lib/stores/index.js';
	import RegistriesView from '$lib/components/mcp/RegistriesView.svelte';
	import { twMerge } from 'tailwind-merge';
	import { page } from '$app/state';
	import { localState } from '$lib/runes/localState.svelte.js';
	import { debounce } from 'es-toolkit';
	import DeploymentsView from '$lib/components/mcp/DeploymentsView.svelte';

	let { data } = $props();
	let query = $state('');

	type View = 'registry' | 'deployments';
	let view = $state<View>((page.url.searchParams.get('view') as View) || 'deployments');

	type LocalStorageViewQuery = Record<View, string>;
	const localStorageViewQuery = localState<LocalStorageViewQuery>(
		'@obot/mcp-servers/search-query',
		{ registry: '', deployments: '' }
	);

	let workspaceId = $derived(data.workspace?.id);
	let isAtLeastPowerUser = $derived(profile.current.groups.includes(Group.POWERUSER));

	afterNavigate(({ from }) => {
		if (browser) {
			// If coming back from a detail page, don't show form - user just created a server
			const comingFromDetailPage =
				from?.url?.pathname.startsWith('/mcp-servers/c/') ||
				from?.url?.pathname.startsWith('/mcp-servers/s/');

			if (comingFromDetailPage) {
				showServerForm = false;
				if (page.url.searchParams.has('new')) {
					const cleanUrl = new URL(page.url);
					cleanUrl.searchParams.delete('new');
					replaceState(cleanUrl, {});
				}
				return;
			}

			const createNewType = page.url.searchParams.get('new') as 'single' | 'multi' | 'remote';
			if (createNewType) {
				selectServerType(createNewType, false);
			} else {
				showServerForm = false;
			}
		}
	});

	beforeNavigate(({ to }) => {
		if (browser && !to?.url.pathname.startsWith('/mcp-servers')) {
			clearQueryFromLocalStorage();
		}
	});

	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerType>>();
	let selectedServerType = $state<SelectServerOption>();

	let showServerForm = $state(false);
	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();

	let urlFilters = $state(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

	function selectServerType(type: SelectServerOption, updateUrl = true) {
		selectedServerType = type;
		selectServerTypeDialog?.close();
		showServerForm = true;
		if (updateUrl) {
			goto(`/mcp-servers?new=${type}`, { replaceState: false });
		}
	}

	function handleFilter(property: string, values: string[]) {
		urlFilters[property] = values;
		setFilterUrlParams(property, values);
	}

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

	function handleClearAllFilters() {
		urlFilters = {};
		clearUrlParams();
	}

	function persistQueryToLocalStorage(view: View, queryValue: string): void {
		if (!localStorageViewQuery.current) {
			return;
		}

		localStorageViewQuery.current[view] = queryValue;
	}

	function clearQueryFromLocalStorage(view?: View): void {
		if (!localStorageViewQuery.current) {
			return;
		}

		if (view) {
			localStorageViewQuery.current[view] = '';
		} else {
			localStorageViewQuery.current = { registry: '', deployments: '' };
		}
	}

	const updateSearchQuery = debounce((value: string) => {
		const newUrl = new URL(page.url);

		setUrlParam(newUrl, 'query', value || null);

		persistQueryToLocalStorage(view, value);
		navigateWithState(newUrl);
	}, 100);

	const duration = PAGE_TRANSITION_DURATION;
	let title = $derived(
		showServerForm ? `Create ${getServerTypeLabelByType(selectedServerType)} Server` : 'MCP Servers'
	);
</script>

<Layout classes={{ navbar: 'bg-surface1' }} showUserLinks {title} showBackButton={showServerForm}>
	<div class="flex min-h-full flex-col gap-8" in:fade>
		{#if showServerForm}
			{@render configureEntryScreen()}
		{:else}
			{@render mainContent()}
		{/if}
	</div>

	{#snippet rightNavActions()}
		{#if isAtLeastPowerUser}
			{@render addServerButton()}
		{/if}
	{/snippet}
</Layout>

{#snippet mainContent()}
	<div
		class="flex flex-col"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div class="bg-surface1 dark:bg-background sticky top-16 left-0 z-20 w-full py-1">
			<div class="mb-2">
				<Search
					class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
					value={query}
					onChange={updateSearchQuery}
					placeholder="Search servers..."
				/>
			</div>
		</div>
		<div class="dark:bg-surface2 bg-background rounded-t-md shadow-sm">
			<div class="flex">
				<button
					class={twMerge('page-tab', view === 'deployments' && 'page-tab-active')}
					onclick={() => switchView('deployments')}
				>
					My Servers
				</button>
				<button
					class={twMerge('page-tab', view === 'registry' && 'page-tab-active')}
					onclick={() => switchView('registry')}
				>
					Registry Entries
				</button>
			</div>

			{#if view === 'registry'}
				<RegistriesView
					id={workspaceId}
					query={localStorageViewQuery.current?.['registry'] || ''}
					{urlFilters}
					onFilter={handleFilter}
					onClearAllFilters={handleClearAllFilters}
					onSort={setSortUrlParams}
					{initSort}
					classes={{
						tableHeader: 'top-31'
					}}
					onConnect={() => {
						mcpServersAndEntries.refreshUserConfiguredServers();
					}}
					onConnectClose={() => {
						switchView('deployments');
					}}
				/>
			{:else if view === 'deployments'}
				<DeploymentsView
					entity="workspace"
					id={workspaceId}
					query={localStorageViewQuery.current?.['deployments'] || ''}
					{urlFilters}
					onFilter={handleFilter}
					onClearAllFilters={handleClearAllFilters}
					onSort={setSortUrlParams}
					{initSort}
				>
					{#snippet noDataContent()}
						<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
							<Server class="text-on-surface1 size-24 opacity-25" />
							<h4 class="text-on-surface1 text-lg font-semibold">No created MCP servers</h4>
							<p class="text-on-surface1 text-sm font-light">
								Looks like you don't have any servers set up yet. <br />
								Connect to a server from <i>"Registry Entries"</i>
								{#if isAtLeastPowerUser}
									or click the button below
								{/if}
								to get started.
							</p>
							{#if isAtLeastPowerUser}
								{@render addServerButton()}
							{:else}
								<button
									class="button-primary flex w-full items-center gap-1 text-sm md:w-fit"
									onclick={() => {
										switchView('registry');
									}}
								>
									<Plus class="size-4" /> Connect To Registry Entry
								</button>
							{/if}
						</div>
					{/snippet}
				</DeploymentsView>
			{/if}
		</div>
	</div>
{/snippet}

{#snippet configureEntryScreen()}
	<div class="flex flex-col gap-6" in:fly={{ x: 100, delay: duration, duration }}>
		<McpServerEntryForm
			type={selectedServerType}
			id={workspaceId}
			entity="workspace"
			onCancel={() => {
				showServerForm = false;
			}}
			onSubmit={async (id, type) => {
				if (type === 'single' || type === 'remote') {
					goto(`/mcp-servers/c/${id}`);
				} else {
					goto(`/mcp-servers/s/${id}`);
				}
			}}
		/>
	</div>
{/snippet}

{#snippet addServerButton()}
	<button
		class="button-primary flex w-full items-center gap-1 text-sm md:w-fit"
		onclick={() => {
			selectServerTypeDialog?.open();
		}}
	>
		<Plus class="size-4" /> Add MCP Server
	</button>
{/snippet}

<McpConfirmDelete
	names={[deletingEntry?.manifest?.name ?? '']}
	show={Boolean(deletingEntry)}
	onsuccess={async () => {
		if (!deletingEntry || !workspaceId) {
			return;
		}

		await ChatService.deleteWorkspaceMCPCatalogEntry(workspaceId, deletingEntry.id);
		await mcpServersAndEntries.refreshAll();
		deletingEntry = undefined;
	}}
	oncancel={() => (deletingEntry = undefined)}
	entity="entry"
	entityPlural="entries"
/>

<McpConfirmDelete
	names={[deletingServer?.manifest?.name ?? '']}
	show={Boolean(deletingServer)}
	onsuccess={async () => {
		if (!deletingServer || !workspaceId) {
			return;
		}

		await ChatService.deleteWorkspaceMCPCatalogServer(workspaceId, deletingServer.id);
		await mcpServersAndEntries.refreshAll();
		deletingServer = undefined;
	}}
	oncancel={() => (deletingServer = undefined)}
	entity="entry"
	entityPlural="entries"
/>

<SelectServerType bind:this={selectServerTypeDialog} onSelectServerType={selectServerType} />

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
