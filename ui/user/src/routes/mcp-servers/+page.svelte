<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import ConnectorsView from '$lib/components/mcp/ConnectorsView.svelte';
	import McpConfirmDelete from '$lib/components/mcp/McpConfirmDelete.svelte';
	import SelectServerType from '$lib/components/mcp/SelectServerType.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		AdminService,
		ChatService,
		Group,
		type LaunchServerType,
		type MCPCatalogServer
	} from '$lib/services';
	import type { MCPCatalogEntry, OrgUser } from '$lib/services/admin/types';
	import { getServerTypeLabelByType } from '$lib/services/chat/mcp.js';
	import { mcpServersAndEntries, profile } from '$lib/stores/index.js';
	import { goto } from '$lib/url';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setFilterUrlParams,
		setSortUrlParams,
		setUrlParam
	} from '$lib/url';
	import { debounce } from 'es-toolkit';
	import { Plus, Server } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';

	let { data } = $props();

	let workspaceId = $derived(data.workspace?.id);
	let isAtLeastPowerUser = $derived(profile.current.groups.includes(Group.POWERUSER));

	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerType>>();

	let users = $state<OrgUser[]>([]);
	let showServerForm = $derived(page.url.searchParams.has('new'));
	let selectedServerType = $derived(page.url.searchParams.get('new') as LaunchServerType);
	let deletingEntry = $state<MCPCatalogEntry>();
	let deletingServer = $state<MCPCatalogServer>();

	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort());

	let registrySearchQuery = $derived(page.url.searchParams.get('query') || '');

	let usersMap = $derived(new Map(users.map((user) => [user.id, user])));

	onMount(async () => {
		users = await AdminService.listUsers();
	});

	function selectServerType(type: LaunchServerType, updateUrl = true) {
		selectServerTypeDialog?.close();
		if (updateUrl) {
			goto(`/mcp-servers?new=${type}`);
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

	const updateSearchQuery = debounce((value: string) => {
		const newUrl = new URL(page.url);

		setUrlParam(newUrl, 'query', value || null);
		navigateWithState(newUrl);
	}, 100);

	const duration = PAGE_TRANSITION_DURATION;
	let title = $derived(
		showServerForm ? `Create ${getServerTypeLabelByType(selectedServerType)} Server` : 'MCP Servers'
	);
</script>

<Layout classes={{ navbar: 'bg-base-200' }} {title} showBackButton={showServerForm}>
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
		<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
			<div class="mb-2">
				<Search
					class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
					value={registrySearchQuery}
					onChange={updateSearchQuery}
					placeholder="Search servers..."
				/>
			</div>
		</div>
		<div class="dark:bg-base-300 bg-base-100 rounded-t-md shadow-sm">
			<ConnectorsView
				id={workspaceId}
				entity="workspace"
				query={registrySearchQuery}
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
						<Server class="text-base-content/80 size-24 opacity-25" />
						<h4 class="text-muted-content text-lg font-semibold">No created MCP servers</h4>
						<p class="text-muted-content text-sm font-light">
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
				goto('/mcp-servers');
			}}
			onSubmit={async (id, type, message) => {
				clearUrlParams(['new']);
				// Determine which query param to use based on the message
				let queryParam = '?launch=true';
				if (message === 'requires-oauth-config') {
					queryParam = '?configure-oauth=true';
				}
				if (type === 'single' || type === 'remote') {
					goto(`/mcp-servers/c/${id}${queryParam}`);
				} else {
					goto(`/mcp-servers/s/${id}${queryParam}`);
				}
			}}
		/>
	</div>
{/snippet}

{#snippet addServerButton()}
	<button
		class="btn btn-primary flex w-full items-center gap-1 text-sm md:w-fit"
		onclick={() => {
			selectServerTypeDialog?.open();
		}}
		id="add-mcp-server-button"
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
