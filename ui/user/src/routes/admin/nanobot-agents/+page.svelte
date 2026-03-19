<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import DeploymentsView from '$lib/components/mcp/DeploymentsView.svelte';
	import type { InitSort } from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, type OrgUser } from '$lib/services';
	import { profile } from '$lib/stores';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		setFilterUrlParams,
		setSortUrlParams,
		setUrlParam,
		goto
	} from '$lib/url';
	import { Server } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	const query = $derived(page.url.searchParams.get('query') || '');
	let users = $state<OrgUser[]>([]);
	const usersMap = $derived(new Map(users.map((user) => [user.id, user])));
	let urlFilters = $state(getTableUrlParamsFilters());
	const initSort = $derived(
		getTableUrlParamsSort({
			property: 'created',
			order: 'desc'
		} as InitSort)
	);
	const isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	onMount(async () => {
		users = await AdminService.listUsersIncludeDeleted();
	});

	function handleFilter(property: string, values: string[]) {
		if (values.length === 0) {
			const rest = { ...urlFilters };
			delete rest[property];
			urlFilters = rest;
		} else {
			urlFilters = { ...urlFilters, [property]: values };
		}
		setFilterUrlParams(property, values);
	}

	function handleClearAllFilters() {
		urlFilters = {};
		clearUrlParams();
	}

	function updateSearchQuery(value: string) {
		const newUrl = new URL(page.url);
		setUrlParam(newUrl, 'query', value || null);
		goto(newUrl, { replaceState: true, noScroll: true, keepFocus: true });
	}
</script>

<Layout title="Nanobot Agents" classes={{ navbar: 'bg-surface1' }}>
	<div
		class="flex min-h-full flex-col"
		in:fly={{ x: 100, delay: PAGE_TRANSITION_DURATION, duration: PAGE_TRANSITION_DURATION }}
		out:fly={{ x: -100, duration: PAGE_TRANSITION_DURATION }}
	>
		<div class="bg-surface1 dark:bg-background sticky top-16 left-0 z-20 w-full py-1">
			<div class="mb-2">
				<Search
					class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
					value={query}
					onChange={updateSearchQuery}
					placeholder="Search nanobot agents..."
				/>
			</div>
		</div>
		<div class="dark:bg-surface2 bg-background rounded-t-md shadow-sm">
			<DeploymentsView
				scope="nanobot"
				readonly={isAdminReadonly}
				{usersMap}
				{query}
				{urlFilters}
				onFilter={handleFilter}
				onClearAllFilters={handleClearAllFilters}
				onSort={setSortUrlParams}
				{initSort}
			>
				{#snippet noDataContent()}
					<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<Server class="text-on-surface1 size-24 opacity-25" />
						{#if !query && Object.keys(urlFilters).length === 0}
							<h4 class="text-on-surface1 text-lg font-semibold">No nanobot agents running</h4>
							<p class="text-on-surface1 text-sm font-light">
								No nanobot-backed MCP server deployments were found.
							</p>
						{:else}
							<h4 class="text-on-surface1 text-lg font-semibold">No results found</h4>
							<p class="text-on-surface1 text-sm font-light">
								No nanobot-backed MCP server deployments match your search or filters.
							</p>
						{/if}
					</div>
				{/snippet}
			</DeploymentsView>
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | Nanobot Agents</title>
</svelte:head>
