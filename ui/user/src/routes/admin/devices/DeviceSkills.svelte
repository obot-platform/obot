<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_SIZE } from '$lib/constants';
	import { AdminService, type DeviceSkillStat, type DeviceSkillStatResponse } from '$lib/services';
	import { getTableUrlParamsSort, replaceState, setSortUrlParams } from '$lib/url';
	import { getSortParams, openUrl } from '$lib/utils';
	import { defaultSkillSort, deviceSkillSortFields } from './constants';
	import { PencilRuler } from '@lucide/svelte';
	import { debounce } from 'es-toolkit';
	import { onMount, untrack } from 'svelte';

	let skillsResp = $state<DeviceSkillStatResponse>({
		items: [],
		total: 0,
		limit: PAGE_SIZE,
		offset: 0
	});
	let pageIndex = $state(
		untrack(() => Math.floor(Number(page.url.searchParams.get('offset') ?? 0) / PAGE_SIZE))
	);
	let loading = $state(true);
	let nameFilter = $state(untrack(() => page.url.searchParams.get('name') ?? ''));

	type Row = DeviceSkillStat & { id: string };

	function isSortProperty(
		property: string | undefined
	): property is keyof typeof deviceSkillSortFields {
		return property != null && Object.hasOwn(deviceSkillSortFields, property);
	}

	let initSort = $derived.by(() => {
		const sort = getTableUrlParamsSort(defaultSkillSort);
		return isSortProperty(sort?.property) ? sort : defaultSkillSort;
	});

	let rows = $derived<Row[]>(
		(skillsResp.items ?? []).map((s) => ({
			...s,
			id: s.name
		}))
	);

	let total = $derived(skillsResp.total ?? 0);
	let lastPageIndex = $derived(total > 0 ? Math.ceil(total / PAGE_SIZE) - 1 : 0);

	onMount(async () => {
		reload();
	});

	function syncUrl() {
		const next = new URL(page.url);
		if (nameFilter) next.searchParams.set('name', nameFilter);
		else next.searchParams.delete('name');
		if (pageIndex > 0) next.searchParams.set('offset', String(pageIndex * PAGE_SIZE));
		else next.searchParams.delete('offset');
		replaceState(next, {});
	}

	async function reload(sort = initSort) {
		loading = true;
		try {
			skillsResp = await AdminService.listDeviceSkills({
				limit: PAGE_SIZE,
				offset: pageIndex * PAGE_SIZE,
				name: nameFilter || undefined,
				...getSortParams(sort, deviceSkillSortFields, defaultSkillSort)
			});
		} finally {
			loading = false;
		}
		syncUrl();
	}

	const updateName = debounce((value: string) => {
		nameFilter = value;
		pageIndex = 0;
		reload();
	}, 200);

	function fetchPage(idx: number) {
		pageIndex = idx;
		reload();
	}

	function handleSort(property: string, order: 'asc' | 'desc') {
		setSortUrlParams(property, order);
		pageIndex = 0;
		reload({ property, order });
	}
</script>

<Search
	value={nameFilter}
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
	onChange={updateName}
	placeholder="Search by skill name..."
/>

{#if loading}
	<Skeleton type="table" />
{:else if total === 0}
	<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
		<PencilRuler class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">No skills observed yet</h4>
		<p class="text-muted-content text-sm font-light">
			Run <code class="font-mono">obot scan</code> from a managed device with SKILL.md files to populate
			this view.
		</p>
	</div>
{:else}
	<Table
		data={rows}
		fields={['name', 'deviceCount', 'userCount', 'observationCount']}
		headers={[
			{ title: 'Name', property: 'name' },
			{ title: 'Devices', property: 'deviceCount' },
			{ title: 'Users', property: 'userCount' },
			{ title: 'Observations', property: 'observationCount' }
		]}
		sortable={['name', 'deviceCount', 'userCount', 'observationCount']}
		{initSort}
		onSort={handleSort}
		onClickRow={(d, isCtrlClick) => {
			openUrl(resolve(`/admin/devices/skills/${encodeURIComponent(d.name)}`), isCtrlClick);
		}}
	>
		{#snippet onRenderColumn(property, d: Row)}
			{d[property as keyof Row] ?? '—'}
		{/snippet}
	</Table>

	{#if total > PAGE_SIZE}
		<Pagination
			{pageIndex}
			{lastPageIndex}
			{total}
			{loading}
			itemLabelSingular="skill"
			onPageChange={fetchPage}
		/>
	{/if}
{/if}
