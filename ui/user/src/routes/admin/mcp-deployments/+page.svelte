<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import DeploymentsView from '$lib/components/mcp/DeploymentsView.svelte';
	import type { InitSort } from '$lib/components/table/Table.svelte';
	import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
	import { UserService, type OrgUser } from '$lib/services';
	import { profile } from '$lib/stores';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setSortUrlParams,
		setFilterUrlParams,
		setUrlParamAndUpdateUrl
	} from '$lib/url';
	import { Server } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';

	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;

	const query = $derived(page.url.searchParams.get('query') || '');

	let users = $state<OrgUser[]>([]);
	let urlFilters = $state(getTableUrlParamsFilters());
	let initSort = $derived.by(() => {
		const defValue = {
			property: 'created',
			order: 'desc'
		} as InitSort;

		return getTableUrlParamsSort(defValue);
	});

	onMount(async () => {
		users = await UserService.listUsersIncludeDeleted();
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
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	const updateSearchQuery = (value: string) => {
		urlFilters = getTableUrlParamsFilters();
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	};
</script>

<Layout classes={{ navbar: 'bg-base-200', container: 'pt-0' }} title="MCP Deployments">
	<div class="flex min-h-full flex-col gap-8" in:fade>
		{@render mainContent()}
	</div>
</Layout>

{#snippet mainContent()}
	<div class="flex min-h-full flex-col">
		<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
			<div class="mb-2">
				<Search
					class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
					value={query}
					onChange={updateSearchQuery}
					placeholder="Search deployments..."
				/>
			</div>
		</div>
		<div class="dark:bg-base-300 bg-base-100 rounded-t-md shadow-sm">
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
		</div>
	</div>
{/snippet}

{#snippet displayNoData()}
	<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
		<Server class="text-muted-content size-24 opacity-25" />
		<h4 class="text-muted-content text-lg font-semibold">No deployments found</h4>
		<p class="text-muted-content text-sm font-light">
			Looks like there aren't any deployments created yet. <br />
			Deployments are created as users connect to MCP servers.
		</p>
	</div>
{/snippet}

<svelte:head>
	<title>Obot | MCP Management | MCP Deployments</title>
</svelte:head>
