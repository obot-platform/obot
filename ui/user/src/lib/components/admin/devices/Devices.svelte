<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_SIZE } from '$lib/constants';
	import { m } from '$lib/i18n';
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

	let devicesToShow = $derived(devicesResp.items);
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

	// Columns the server can sort across the full result set. MCP/Skills/
	// Plugins/Clients are derived from preloaded child-array lengths and
	// would need server-side aggregation to sort properly (obot#6739), so
	// they're intentionally left out here -- Table's own client-side sort
	// still orders whatever page is currently loaded for those columns.
	const SERVER_SORT_FIELDS: Record<string, string> = {
		short_device_id: 'device_id',
		os_arch: 'os',
		username: 'user',
		scannedAt: 'scanned_at'
	};

	function apiSortFor(sort?: { property: string; order: 'asc' | 'desc' }) {
		if (!sort) return {};
		const sortBy = SERVER_SORT_FIELDS[sort.property];
		return sortBy ? { sortBy, sortOrder: sort.order } : {};
	}

	let total = $derived(devicesResp.total ?? 0);
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
		fetchPage(0);
	}, 100);

	// Sort/search changes narrow or reorder the whole result set, so the
	// current page offset no longer means anything -- always refetch page 0.
	function handleSort(property: string, order: 'asc' | 'desc') {
		setSortUrlParams(property, order);
		const { sortBy, sortOrder } = apiSortFor({ property, order });
		if (!sortBy) return; // count columns: Table's client-side sort already handles the current page
		fetchPage(0, { sortBy, sortOrder });
	}

	// Guards against overlapping requests (e.g. a sort click right after a
	// search keystroke): only the response from the most recently started
	// fetch is committed, so a slower, older request can't clobber newer
	// results or clear `loading` out from under a still-pending request.
	let fetchGeneration = 0;

	async function fetchPage(
		idx: number,
		sortOverride?: { sortBy?: string; sortOrder?: 'asc' | 'desc' }
	) {
		const generation = ++fetchGeneration;
		loading = true;
		try {
			const { sortBy, sortOrder } = sortOverride ?? apiSortFor(initSort);
			const response = await UserService.listDeviceScans({
				limit: PAGE_SIZE,
				offset: idx * PAGE_SIZE,
				groupByDevice: true,
				query: query || undefined,
				sortBy,
				sortOrder
			});
			if (generation !== fetchGeneration) return;
			devicesResp = response;
			pageIndex = idx;
			syncUrl();
		} finally {
			if (generation === fetchGeneration) loading = false;
		}
	}

	const hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
	const filterable = ['os_arch'];
</script>

<Search
	value={query}
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
	onChange={updateQuery}
	placeholder={m.inventory_enforcement_devices_search_placeholder()}
/>

{#if loading || parentLoading}
	<Skeleton type="table" classes={{ header: 'h-14' }} />
{:else if total === 0 && !query}
	<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
		<Laptop class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">
			{m.inventory_enforcement_devices_none_scanned()}
		</h4>
		<p class="text-muted-content text-sm font-light">
			{m.inventory_enforcement_devices_run_scan_prefix()}<code class="font-mono">obot scan</code
			>{m.inventory_enforcement_devices_run_scan_suffix()}
		</p>
	</div>
{:else}
	<Table
		data={rows}
		noDataMessage={m.core_no_results_found()}
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
			{ title: m.audit_usage_audit_logs_device(), property: 'short_device_id' },
			{ title: m.inventory_enforcement_devices_col_os_arch(), property: 'os_arch' },
			{ title: m.core_col_user(), property: 'username' },
			{ title: 'MCP', property: 'mcp_count' },
			{ title: m.inventory_enforcement_skills_tab(), property: 'skill_count' },
			{ title: m.inventory_enforcement_devices_col_plugins(), property: 'plugin_count' },
			{ title: m.audit_usage_exports_filter_title_clients(), property: 'client_count' },
			{ title: m.inventory_enforcement_devices_col_last_scanned(), property: 'scannedAt' }
		]}
		sortable={[
			'short_device_id',
			'os_arch',
			'username',
			'mcp_count',
			'skill_count',
			'plugin_count',
			'client_count',
			'scannedAt'
		]}
		manualSortFields={Object.keys(SERVER_SORT_FIELDS)}
		{filterable}
		onClickRow={(d, isCtrlClick) => {
			openUrl(resolve(`/inventory/devices/${d.deviceID}`), isCtrlClick);
		}}
		{initSort}
		onFilter={setFilterUrlParams}
		onClearAllFilters={() => clearUrlParams(filterable)}
		onSort={handleSort}
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
			itemCountLabel={(count) =>
				count === 1
					? m.inventory_enforcement_devices_count_one({ count })
					: m.inventory_enforcement_devices_count_other({ count })}
			onPageChange={fetchPage}
		/>
	{/if}
{/if}
