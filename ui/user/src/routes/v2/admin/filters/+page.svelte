<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import { BookOpenText, ChevronLeft, Plus, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import Confirm from '$lib/components/Confirm.svelte';
	import { DEFAULT_MCP_CATALOG_ID, PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { onMount } from 'svelte';
	import {
		fetchMcpServerAndEntries,
		initMcpServerAndEntries
	} from '$lib/context/admin/mcpServerAndEntries.svelte';
	import { AdminService } from '$lib/services/index.js';
	import FilterForm from '$lib/components/admin/FilterForm.svelte';

	type Filter = {
		id: string;
		displayName: string;
		urls: string[];
		servers: string[];
	};

	const defaultCatalogId = DEFAULT_MCP_CATALOG_ID;
	let showCreateFilter = $state(false);
	let filterToDelete = $state<Filter>();

	initMcpServerAndEntries();
	let filters = $state<Filter[]>([]);

	onMount(() => {
		const url = new URL(window.location.href);
		const queryParams = new URLSearchParams(url.search);
		if (queryParams.get('new')) {
			showCreateFilter = true;
		}
	});

	function handleNavigation(url: string) {
		goto(url, { replaceState: false });
	}

	// async function navigateToCreated(filterId: string) {
	// 	showCreateFilter = false;
	// 	goto(`/v2/admin/filters/${filterId}`, { replaceState: false });
	// }

	const duration = PAGE_TRANSITION_DURATION;
	onMount(async () => {
		fetchMcpServerAndEntries(defaultCatalogId);
	});
</script>

<Layout>
	<div
		class="my-4 h-full w-full"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if showCreateFilter}
			{@render createFilterScreen()}
		{:else}
			<div
				class="flex flex-col gap-8"
				in:fly={{ x: 100, delay: duration, duration }}
				out:fly={{ x: -100, duration }}
			>
				<div class="flex items-center justify-between">
					<h1 class="text-2xl font-semibold">Filters</h1>
					{#if filters.length > 0}
						<div class="relative flex items-center gap-4">
							{@render addFilterButton()}
						</div>
					{/if}
				</div>
				{#if filters.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<BookOpenText class="size-24 text-gray-200 dark:text-gray-900" />
						<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
							No created filters
						</h4>
						<p class="text-sm font-light text-gray-400 dark:text-gray-600">
							Looks like you don't have any filters created yet. <br />
							Click the button below to get started.
						</p>

						{@render addFilterButton()}
					</div>
				{:else}
					<Table
						data={filters}
						fields={['displayName', 'servers']}
						onSelectRow={(d) => {
							handleNavigation(`/v2/admin/filters/${d.id}`);
						}}
						headers={[
							{
								title: 'Name',
								property: 'displayName'
							}
						]}
					>
						{#snippet actions(d)}
							<button
								class="icon-button hover:text-red-500"
								onclick={(e) => {
									e.stopPropagation();
									filterToDelete = d;
								}}
								use:tooltip={'Delete Filter'}
							>
								<Trash2 class="size-4" />
							</button>
						{/snippet}
						{#snippet onRenderColumn(property, d)}
							{#if property === 'servers'}
								{@const count = d.servers.length}
								{count ? count : '-'}
							{:else}
								{d[property as keyof typeof d]}
							{/if}
						{/snippet}
					</Table>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

{#snippet addFilterButton()}
	<button
		class="button-primary flex items-center gap-1 text-sm"
		onclick={() => (showCreateFilter = true)}
	>
		<Plus class="size-4" /> Add New Filter
	</button>
{/snippet}

{#snippet createFilterScreen()}
	<div
		class="h-full w-full"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<FilterForm>
			{#snippet topContent()}
				<button
					onclick={() => (showCreateFilter = false)}
					class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
				>
					<ChevronLeft class="size-6" />
					Filters
				</button>
			{/snippet}
		</FilterForm>
	</div>
{/snippet}

<Confirm
	msg="Are you sure you want to delete this filter?"
	show={Boolean(filterToDelete)}
	onsuccess={async () => {
		if (!filterToDelete) return;
		await AdminService.deleteAccessControlRule(filterToDelete.id);
		// filters = await [fetchFilters]
		filterToDelete = undefined;
	}}
	oncancel={() => (filterToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Filters</title>
</svelte:head>
