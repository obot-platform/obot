<script lang="ts">
	import McpServerEntryForm from '$lib/components/admin/McpServerEntryForm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { ChatService, Group, type MCPCatalogServer } from '$lib/services';
	import type { MCPCatalogEntry } from '$lib/services/admin/types';
	import { LoaderCircle, Plus, Server } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { afterNavigate } from '$app/navigation';
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
		setSortUrlParams
	} from '$lib/url';
	import { profile } from '$lib/stores/index.js';
	import {
		fetchUserMcpServerAndEntries,
		getUserMcpServerAndEntries,
		initUserMcpServerAndEntries
	} from '$lib/context/mcpServerAndEntries.svelte.js';
	import RegistriesView from '$lib/components/mcp/RegistriesView.svelte';

	let { data } = $props();
	let query = $state('');
	let workspaceId = $derived(data.workspace?.id);
	let isAtLeastPowerUser = $derived(profile.current.groups.includes(Group.POWERUSER));

	initUserMcpServerAndEntries();
	const mcpServerAndEntries = getUserMcpServerAndEntries();

	onMount(() => {
		fetchUserMcpServerAndEntries(mcpServerAndEntries);
	});

	afterNavigate(({ to }) => {
		if (browser && to?.url) {
			const serverId = to.url.searchParams.get('id');
			const createNewType = to.url.searchParams.get('new') as 'single' | 'multi' | 'remote';
			if (createNewType) {
				selectServerType(createNewType, false);
			} else if (!serverId && (selectedEntryServer || showServerForm)) {
				selectedEntryServer = undefined;
				showServerForm = false;
			}
		}
	});

	let selectServerTypeDialog = $state<ReturnType<typeof SelectServerType>>();
	let selectedServerType = $state<SelectServerOption>();
	let selectedEntryServer = $state<MCPCatalogEntry | MCPCatalogServer>();

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

	function handleClearAllFilters() {
		urlFilters = {};
		clearUrlParams();
	}

	const duration = PAGE_TRANSITION_DURATION;
	let title = $derived(
		showServerForm ? `Create ${getServerTypeLabelByType(selectedServerType)} Server` : 'MCP Servers'
	);
</script>

<Layout showUserLinks {title} showBackButton={showServerForm}>
	<div class="flex flex-col gap-8 pt-4 pb-8" in:fade>
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
		class="flex flex-col gap-4 md:gap-8"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div class="flex flex-col gap-2">
			<Search
				class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
				onChange={(val) => (query = val)}
				placeholder="Search servers..."
			/>

			{#if mcpServerAndEntries.loading}
				<div class="my-2 flex items-center justify-center">
					<LoaderCircle class="size-6 animate-spin" />
				</div>
			{:else}
				<RegistriesView
					entity="workspace"
					id={workspaceId}
					{query}
					{urlFilters}
					onFilter={handleFilter}
					onClearAllFilters={handleClearAllFilters}
					onSort={setSortUrlParams}
					{initSort}
					classes={{
						tableHeader: 'top-16'
					}}
				>
					{#snippet noDataContent()}
						<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
							<Server class="text-on-surface1 size-24 opacity-25" />
							<h4 class="text-on-surface1 text-lg font-semibold">No created MCP servers</h4>
							{#if isAtLeastPowerUser}
								<p class="text-on-surface1 text-sm font-light">
									Looks like you don't have any servers created yet. <br />
									Click the button below to get started.
								</p>

								{@render addServerButton()}
							{:else}
								<p class="text-on-surface1 text-sm font-light">
									Looks like there aren't any servers available to connect to yet! <br />
								</p>
							{/if}
						</div>
					{/snippet}
				</RegistriesView>
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
				selectedEntryServer = undefined;
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
		await fetchUserMcpServerAndEntries(mcpServerAndEntries);
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
		await fetchUserMcpServerAndEntries(mcpServerAndEntries);
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
