<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_SIZE } from '$lib/constants';
	import {
		UserService,
		type DeviceScan,
		type DeviceScanResponse,
		type OrgUser
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		replaceState,
		setFilterUrlParams,
		setSortUrlParams
	} from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Laptop } from '@lucide/svelte';
	import { debounce } from 'es-toolkit';
	import { onMount, untrack } from 'svelte';

	interface Props {
		devices?: DeviceScanResponse;
		users?: OrgUser[];
		loading?: boolean;
	}

	let { devices, users, loading: parentLoading }: Props = $props();

	let usersResp = $state<OrgUser[]>(untrack(() => users ?? []));
	let devicesResp = $state<DeviceScanResponse>(
		untrack(() => devices ?? { items: [], total: 0, limit: PAGE_SIZE, offset: 0 })
	);
	let pageIndex = $state(
		untrack(() => Math.floor(Number(page.url.searchParams.get('offset') ?? 0) / PAGE_SIZE))
	);
	let loading = $state(false);
	let query = $state(untrack(() => page.url.searchParams.get('query') ?? ''));
	let filters = $derived.by(() => {
		const f = getTableUrlParamsFilters();
		delete f.start;
		delete f.end;
		delete f.offset;
		return f;
	});

	type Row = DeviceScan & {
		short_device_id: string;
		os_arch: string;
		mcp_count: number;
		skill_count: number;
		plugin_count: number;
		client_count: number;
	};

	const userById = $derived<Map<string, OrgUser>>(new Map(usersResp.map((u) => [u.id, u])));

	function userDisplay(u: OrgUser): string {
		return u.displayName ?? u.email ?? u.username ?? u.id;
	}

	let onlyShowMyDevices = $derived(
		!page.url.pathname.startsWith('/admin') && profile.current.hasAdminAccess?.()
	);
	let devicesToShow = $derived(
		onlyShowMyDevices
			? devicesResp.items?.filter((d) => d.submittedBy === profile.current.id)
			: devicesResp.items
	);
	let rows = $derived<Row[]>(
		(devicesToShow ?? []).map((s) => {
			const u = s.submittedBy ? userById.get(s.submittedBy) : undefined;
			return {
				...s,
				username: u ? userDisplay(u) : s.username,
				short_device_id: (s.deviceID ?? '').slice(0, 12),
				os_arch: `${s.os} / ${s.arch}`,
				mcp_count: s.mcpServers?.length ?? 0,
				skill_count: s.skills?.length ?? 0,
				plugin_count: s.plugins?.length ?? 0,
				client_count: s.clients?.length ?? 0
			};
		})
	);

	let filteredRows = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return rows;
		return rows.filter((r) => {
			if ((r.deviceID ?? '').toLowerCase().includes(q)) return true;
			if ((r.username ?? '').toLowerCase().includes(q)) return true;
			const u = r.submittedBy ? userById.get(r.submittedBy) : undefined;
			if (u && userDisplay(u).toLowerCase().includes(q)) return true;
			return false;
		});
	});

	let total = $derived(onlyShowMyDevices ? (devicesToShow?.length ?? 0) : (devicesResp.total ?? 0));
	let lastPageIndex = $derived(total > 0 ? Math.ceil(total / PAGE_SIZE) - 1 : 0);
	let initSort = $derived(getTableUrlParamsSort({ property: 'scannedAt', order: 'desc' }));

	onMount(async () => {
		if (!devices) {
			fetchPage(pageIndex);
		}

		if (!users) {
			UserService.listUsers().then((response) => {
				usersResp = response;
			});
		}
	});

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
			devicesResp = await UserService.listDeviceScans({
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

	const hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
	const filterable = ['os_arch'];
</script>

<Search
	value={query}
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
	onChange={updateQuery}
	placeholder="Search by device ID or user..."
/>

{#if loading || parentLoading}
	<Skeleton type="table" classes={{ header: 'h-14' }} />
{:else if total === 0}
	<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
		<Laptop class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">No devices scanned yet</h4>
		<p class="text-muted-content text-sm font-light">
			Run <code class="font-mono">obot scan</code> from a managed device to populate this view.
		</p>
	</div>
{:else}
	<Table
		data={filteredRows}
		fields={[
			'short_device_id',
			'os_arch',
			...(hasAdminAccess ? ['username'] : []),
			'mcp_count',
			'skill_count',
			'plugin_count',
			'client_count',
			'scannedAt'
		]}
		headers={[
			{ title: 'Device', property: 'short_device_id' },
			{ title: 'OS / Arch', property: 'os_arch' },
			{ title: 'User', property: 'username' },
			{ title: 'MCP', property: 'mcp_count' },
			{ title: 'Skills', property: 'skill_count' },
			{ title: 'Plugins', property: 'plugin_count' },
			{ title: 'Clients', property: 'client_count' },
			{ title: 'Last Scanned', property: 'scannedAt' }
		]}
		sortable={['short_device_id', 'os_arch', 'username', 'scannedAt']}
		{filterable}
		onClickRow={(d, isCtrlClick) => {
			const prefix = hasAdminAccess ? '/admin' : '';
			openUrl(resolve(`${prefix}/devices/${d.deviceID}`), isCtrlClick);
		}}
		{initSort}
		onFilter={setFilterUrlParams}
		onClearAllFilters={() => clearUrlParams(filterable)}
		onSort={setSortUrlParams}
		{filters}
	>
		{#snippet onRenderColumn(property, d: Row)}
			{#if property === 'short_device_id'}
				<span title={d.deviceID}>{d.short_device_id}</span>
			{:else if property === 'username'}
				{@const u = d.submittedBy ? userById.get(d.submittedBy) : undefined}
				{#if u}
					<div class="flex items-center gap-2">
						<div class="size-5 shrink-0 overflow-hidden rounded-full bg-base-100 dark:bg-base-300">
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
			{:else if property === 'scannedAt'}
				<span>{formatTimeAgo(d.scannedAt).relativeTime}</span>
			{:else}
				{d[property as keyof Row] ?? '—'}
			{/if}
		{/snippet}
	</Table>

	{#if total > PAGE_SIZE}
		<Pagination
			{pageIndex}
			{lastPageIndex}
			{total}
			{loading}
			itemLabelSingular="device"
			onPageChange={fetchPage}
		/>
	{/if}
{/if}
