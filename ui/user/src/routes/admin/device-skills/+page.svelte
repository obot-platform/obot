<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, type DeviceSkillStat, type DeviceSkillStatResponse } from '$lib/services';
	import { replaceState } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { debounce } from 'es-toolkit';
	import { PencilRuler } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const PAGE_SIZE = untrack(() => data?.pageSize ?? 50);

	let skillsResp = $state<DeviceSkillStatResponse>(
		untrack(() => data?.skills ?? { items: [], total: 0, limit: PAGE_SIZE, offset: 0 })
	);
	let pageIndex = $state(untrack(() => Math.floor((data?.skills?.offset ?? 0) / PAGE_SIZE)));
	let loading = $state(false);
	let nameFilter = $state(untrack(() => page.url.searchParams.get('name') ?? ''));

	type Row = DeviceSkillStat & { id: string };

	let rows = $derived<Row[]>(
		(skillsResp.items ?? []).map((s) => ({
			...s,
			id: s.name
		}))
	);

	let total = $derived(skillsResp.total ?? 0);
	let lastPageIndex = $derived(total > 0 ? Math.ceil(total / PAGE_SIZE) - 1 : 0);

	function syncUrl() {
		const next = new URL(page.url);
		if (nameFilter) next.searchParams.set('name', nameFilter);
		else next.searchParams.delete('name');
		if (pageIndex > 0) next.searchParams.set('offset', String(pageIndex * PAGE_SIZE));
		else next.searchParams.delete('offset');
		replaceState(next, {});
	}

	async function reload() {
		loading = true;
		try {
			skillsResp = await AdminService.listDeviceSkills({
				limit: PAGE_SIZE,
				offset: pageIndex * PAGE_SIZE,
				name: nameFilter || undefined
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

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | Device Skills</title>
</svelte:head>

<Layout title="Device Skills">
	<div
		class="flex h-full w-full flex-col gap-4"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		<Search
			value={nameFilter}
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
			onChange={updateName}
			placeholder="Search by skill name..."
		/>

		{#if total === 0 && !loading}
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
				onClickRow={(d, isCtrlClick) => {
					openUrl(resolve(`/admin/device-skills/${encodeURIComponent(d.name)}`), isCtrlClick);
				}}
			>
				{#snippet onRenderColumn(property, d: Row)}
					{#if property === 'name'}
						<span class="font-medium">{d.name}</span>
					{:else}
						{d[property as keyof Row]}
					{/if}
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
	</div>
</Layout>
