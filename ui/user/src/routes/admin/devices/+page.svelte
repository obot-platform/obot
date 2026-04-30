<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { AdminService, type DeviceScan, type DeviceScanList, type OrgUser } from '$lib/services';
	import { formatTimeAgo } from '$lib/time';
	import { replaceState } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { debounce } from 'es-toolkit';
	import { ChevronsLeft, ChevronsRight, Laptop } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const PAGE_SIZE = untrack(() => data?.pageSize ?? 50);

	let devicesResp = $state<DeviceScanList>(
		untrack(() => data?.devices ?? { items: [], total: 0, limit: PAGE_SIZE, offset: 0 })
	);
	let pageIndex = $state(untrack(() => Math.floor((data?.devices?.offset ?? 0) / PAGE_SIZE)));
	let loading = $state(false);
	let query = $state(untrack(() => page.url.searchParams.get('query') ?? ''));

	type Row = DeviceScan & {
		short_device_id: string;
		os_arch: string;
		mcp_count: number;
		skill_count: number;
		plugin_count: number;
		client_count: number;
		scanned_relative: string;
	};

	let rows = $derived<Row[]>(
		(devicesResp.items ?? []).map((s) => ({
			...s,
			short_device_id: (s.device_id ?? '').slice(0, 12),
			os_arch: `${s.os} / ${s.arch}`,
			mcp_count: s.mcp_servers?.length ?? 0,
			skill_count: s.skills?.length ?? 0,
			plugin_count: s.plugins?.length ?? 0,
			client_count: s.clients?.length ?? 0,
			scanned_relative: formatTimeAgo(s.scanned_at).relativeTime
		}))
	);

	let userById = $state<Record<string, OrgUser | null>>({});

	$effect(() => {
		for (const r of rows) {
			const id = r.submitted_by;
			if (!id || id in userById) continue;
			userById[id] = null;
			AdminService.getUser(id, { dontLogErrors: true })
				.then((u) => {
					userById[id] = u;
				})
				.catch(() => {
					userById[id] = null;
				});
		}
	});

	function userDisplay(u: OrgUser): string {
		return u.displayName ?? u.email ?? u.username ?? u.id;
	}

	let filteredRows = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return rows;
		return rows.filter((r) => {
			if ((r.device_id ?? '').toLowerCase().includes(q)) return true;
			if ((r.username ?? '').toLowerCase().includes(q)) return true;
			const u = r.submitted_by ? userById[r.submitted_by] : null;
			if (u && userDisplay(u).toLowerCase().includes(q)) return true;
			return false;
		});
	});

	let total = $derived(devicesResp.total ?? 0);
	let lastPageIndex = $derived(total > 0 ? Math.ceil(total / PAGE_SIZE) - 1 : 0);

	function syncUrl() {
		const next = new URL(page.url);
		if (query) next.searchParams.set('query', query);
		else next.searchParams.delete('query');
		if (pageIndex > 0) next.searchParams.set('offset', String(pageIndex * PAGE_SIZE));
		else next.searchParams.delete('offset');
		replaceState(next, {});
	}

	const updateQuery = debounce((v: string) => {
		query = v;
		syncUrl();
	}, 100);

	async function fetchPage(idx: number) {
		loading = true;
		try {
			devicesResp = await AdminService.listDeviceScans({
				limit: PAGE_SIZE,
				offset: idx * PAGE_SIZE,
				groupByDevice: true
			});
			pageIndex = idx;
			syncUrl();
		} finally {
			loading = false;
		}
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | Devices</title>
</svelte:head>

<Layout title="Devices">
	<div
		class="flex h-full w-full flex-col gap-4"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if total === 0 && !loading}
			<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
				<Laptop class="text-on-surface1 size-24 opacity-50" />
				<h4 class="text-on-surface1 text-lg font-semibold">No devices scanned yet</h4>
				<p class="text-on-surface1 text-sm font-light">
					Run <code class="font-mono">obot scan</code> from a managed device to populate this view.
				</p>
			</div>
		{:else}
			<Search
				value={query}
				class="dark:bg-surface1 dark:border-surface3 bg-background border border-transparent shadow-sm"
				onChange={updateQuery}
				placeholder="Search by device ID or user..."
			/>

			<Table
				data={filteredRows}
				fields={[
					'short_device_id',
					'os_arch',
					'username',
					'mcp_count',
					'skill_count',
					'plugin_count',
					'client_count',
					'scanned_relative'
				]}
				headers={[
					{ title: 'Device', property: 'short_device_id' },
					{ title: 'OS / Arch', property: 'os_arch' },
					{ title: 'User', property: 'username' },
					{ title: 'MCP', property: 'mcp_count' },
					{ title: 'Skills', property: 'skill_count' },
					{ title: 'Plugins', property: 'plugin_count' },
					{ title: 'Clients', property: 'client_count' },
					{ title: 'Last Scanned', property: 'scanned_relative' }
				]}
				sortable={['short_device_id', 'os_arch', 'username']}
				filterable={['os_arch']}
				onClickRow={(d, isCtrlClick) => {
					openUrl(resolve(`/admin/devices/${d.device_id}`), isCtrlClick);
				}}
			>
				{#snippet onRenderColumn(property, d: Row)}
					{#if property === 'short_device_id'}
						<span class="font-mono text-xs" title={d.device_id}>{d.short_device_id}</span>
					{:else if property === 'username'}
						{@const u = d.submitted_by ? userById[d.submitted_by] : null}
						{#if u}
							<div class="flex items-center gap-2">
								<div
									class="size-5 shrink-0 overflow-hidden rounded-full bg-gray-50 dark:bg-gray-600"
								>
									{#if u.iconURL}
										<img
											src={u.iconURL}
											class="h-full w-full object-cover"
											alt=""
											referrerpolicy="no-referrer"
										/>
									{/if}
								</div>
								<span>{userDisplay(u)}</span>
							</div>
						{:else if d.username}
							<span class="font-mono text-xs">{d.username}</span>
						{:else}
							—
						{/if}
					{:else}
						{d[property as keyof Row] ?? '—'}
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
						{pageIndex + 1} of {lastPageIndex + 1} · {total} device{total === 1 ? '' : 's'}
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
