<script lang="ts">
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		AdminService,
		ChatService,
		Group,
		type LaunchServerType,
		type MCPCatalogServer
	} from '$lib/services';
	import type { MCPCatalogEntry, OrgUser } from '$lib/services/admin/types';
	import { Plus, Server } from 'lucide-svelte';
	import { fade, fly } from 'svelte/transition';
	import { goto, replaceState } from '$lib/url';
	import { beforeNavigate, afterNavigate } from '$app/navigation';
	import { browser } from '$app/environment';
	import Search from '$lib/components/Search.svelte';
	import SelectServerType from '$lib/components/mcp/SelectServerType.svelte';
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
	import { page } from '$app/state';
	import { localState } from '$lib/runes/localState.svelte.js';
	import { debounce } from 'es-toolkit';
	import ConnectorsView from '$lib/components/mcp/ConnectorsView.svelte';
	import { onMount } from 'svelte';

	let { data } = $props();

	type LocalStorageViewQuery = Record<'registry', string>;
	const localStorageViewQuery = localState<LocalStorageViewQuery>(
		'@obot/mcp-servers/search-query',
		{ registry: '' }
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
	let selectedServerType = $state<LaunchServerType>();

	let users = $state<OrgUser[]>([]);
	let showServerForm = $state(false);
	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();

	let urlFilters = $state(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));

	onMount(async () => {
		users = await AdminService.listUsers();
	});

	function selectServerType(type: LaunchServerType, updateUrl = true) {
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
		goto(url, { replaceState: true, noScroll: true, keepFocus: true });
	}

	function handleClearAllFilters() {
		urlFilters = {};
		clearUrlParams();
	}

	function persistQueryToLocalStorage(queryValue: string): void {
		if (!localStorageViewQuery.current) {
			return;
		}

		localStorageViewQuery.current.registry = queryValue;
	}

	function clearQueryFromLocalStorage(): void {
		if (!localStorageViewQuery.current) {
			return;
		}

		localStorageViewQuery.current.registry = '';
	}

	const updateSearchQuery = debounce((value: string) => {
		const newUrl = new URL(page.url);

		setUrlParam(newUrl, 'query', value || null);

		persistQueryToLocalStorage(value);
		navigateWithState(newUrl);
	}, 100);

	const duration = PAGE_TRANSITION_DURATION;
	let title = $derived(
		showServerForm ? `Create ${getServerTypeLabelByType(selectedServerType)} Server` : 'MCP Servers'
	);
</script>

<Layout classes={{ navbar: 'bg-surface1' }} {title} showBackButton={showServerForm}>
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
					value={localStorageViewQuery.current?.registry || ''}
					onChange={updateSearchQuery}
					placeholder="Search servers..."
				/>
			</div>
		</div>
		<div class="dark:bg-surface2 bg-background rounded-t-md shadow-sm">
			<ConnectorsView
				id={workspaceId}
				entity="workspace"
				query={localStorageViewQuery.current?.['registry'] || ''}
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
				{usersMap}
			>
				{#snippet noDataContent()}
					<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<Server class="text-on-surface1 size-24 opacity-25" />
						<h4 class="text-on-surface1 text-lg font-semibold">No created MCP servers</h4>
						<p class="text-on-surface1 text-sm font-light">
							{#if isAtLeastPowerUser}
								Looks like you don't have any servers created yet. <br />
								Click the button below to get started.
							{:else}
								There are no servers available to connect to yet. <br />
								Please check back later or contact your administrator.
							{/if}
						</p>

						{#if isAtLeastPowerUser}
							{@render addServerButton()}
						{/if}
					</div>
				{/snippet}
			</ConnectorsView>
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
					goto(`/mcp-servers/c/${id}?launch=true`);
				} else {
					goto(`/mcp-servers/s/${id}?launch=true`);
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

<SelectServerType
	bind:this={selectServerTypeDialog}
	onSelectServerType={selectServerType}
	entity="workspace"
/>

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
