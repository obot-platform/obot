<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, type DeviceSkillStat, type DeviceSkillStatList } from '$lib/services';
	import { replaceState } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { debounce } from 'es-toolkit';
	import { ChevronsLeft, ChevronsRight, PencilRuler } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const PAGE_SIZE = untrack(() => data?.pageSize ?? 50);

	let skillsResp = $state<DeviceSkillStatList>(
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
	<title>Obot | Skills</title>
</svelte:head>

<Layout title="Skills">
	<div
		class="flex h-full w-full flex-col gap-4"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		<Search
			value={nameFilter}
			class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
			onChange={updateName}
			placeholder="Search by skill name..."
		/>

		{#if total === 0 && !loading}
			<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
				<PencilRuler class="text-on-surface1 size-24 opacity-50" />
				<h4 class="text-on-surface1 text-lg font-semibold">No skills observed yet</h4>
				<p class="text-on-surface1 text-sm font-light">
					Run <code class="font-mono">obot scan</code> from a managed device with SKILL.md files to populate
					this view.
				</p>
			</div>
		{:else}
			<Table
				data={rows}
				fields={['name', 'device_count', 'user_count', 'observation_count']}
				headers={[
					{ title: 'Name', property: 'name' },
					{ title: 'Devices', property: 'device_count' },
					{ title: 'Users', property: 'user_count' },
					{ title: 'Observations', property: 'observation_count' }
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
				<div class="flex items-center justify-center gap-4 pt-2">
					<button
						class="button-text flex items-center gap-1 text-xs"
						disabled={pageIndex === 0 || loading}
						onclick={() => fetchPage(pageIndex - 1)}
					>
						<ChevronsLeft class="size-4" /> Previous
					</button>
					<p class="text-on-surface1 text-xs">
						{pageIndex + 1} of {lastPageIndex + 1} · {total} skill{total === 1 ? '' : 's'}
					</p>
					<button
						class="button-text flex items-center gap-1 text-xs"
						disabled={pageIndex >= lastPageIndex || loading}
						onclick={() => fetchPage(pageIndex + 1)}
					>
						Next <ChevronsRight class="size-4" />
					</button>
				</div>
			{/if}
		{/if}
	</div>
</Layout>
