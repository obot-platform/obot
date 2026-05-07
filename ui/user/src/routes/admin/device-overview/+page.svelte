<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import DonutGraph, { type DonutDatum } from '$lib/components/graph/DonutGraph.svelte';
	import StackedTimeline from '$lib/components/graph/StackedTimeline.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		AdminService,
		type DeviceClientStat,
		type DeviceMCPServerStat,
		type DeviceScanStats,
		type DeviceSkillStat
	} from '$lib/services';
	import { replaceState } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { DEFAULT_WINDOW_MS } from './constants';
	import {
		ChevronRight,
		Laptop,
		MonitorCheck,
		PencilRuler,
		ScanLine,
		Server,
		Users
	} from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();

	const TOP_N = 10;
	const PALETTE = [
		'#4575b4',
		'#74add1',
		'#abd9e9',
		'#e0f3f8',
		'#fee090',
		'#fdae61',
		'#f46d43',
		'#d73027',
		'#a50026',
		'#7f3b08'
	];
	const OTHER_COLOR = 'var(--surface3, #6b7280)';

	let stats = $state<DeviceScanStats | null>(untrack(() => data?.stats ?? null));
	let range = $state<{ start: string; end: string }>(
		untrack(
			() =>
				data?.range ?? {
					start: new Date(Date.now() - DEFAULT_WINDOW_MS).toISOString(),
					end: new Date().toISOString()
				}
		)
	);
	let loading = $state(false);

	type Drilldown = 'mcp' | 'skill' | undefined;

	type TopBucket = {
		key: string;
		label: string;
		value: number;
		color: string;
		isOther: boolean;
		otherCount?: number;
		drilldown: Drilldown;
	};

	function buildTop<T>(
		items: T[] | null | undefined,
		key: (t: T) => string,
		label: (t: T) => string,
		value: (t: T) => number,
		drilldown: Drilldown = undefined
	): TopBucket[] {
		const all = (items ?? []).filter((t) => value(t) > 0);
		const sorted = [...all].sort((a, b) => value(b) - value(a));
		const top = sorted.slice(0, TOP_N).map<TopBucket>((t, i) => ({
			key: key(t),
			label: label(t),
			value: value(t),
			color: PALETTE[i] ?? OTHER_COLOR,
			isOther: false,
			drilldown
		}));
		const tail = sorted.slice(TOP_N);
		const otherSum = tail.reduce((s, t) => s + value(t), 0);
		if (otherSum > 0) {
			top.push({
				key: '__other__',
				label: 'Other',
				value: otherSum,
				color: OTHER_COLOR,
				isOther: true,
				otherCount: tail.length,
				drilldown: undefined
			});
		}
		return top;
	}

	let clientBuckets = $derived(
		buildTop<DeviceClientStat>(
			stats?.clients,
			(c) => c.name,
			(c) => c.name,
			(c) => c.device_count
		)
	);

	let mcpBuckets = $derived(
		buildTop<DeviceMCPServerStat>(
			stats?.mcp_servers,
			(m) => m.config_hash,
			(m) => m.name?.trim() || '(unnamed)',
			(m) => m.device_count,
			'mcp'
		)
	);

	let skillBuckets = $derived(
		buildTop<DeviceSkillStat>(
			stats?.skills,
			(s) => s.name,
			(s) => s.name,
			(s) => s.device_count,
			'skill'
		)
	);

	function bucketsToDonut(buckets: TopBucket[]): DonutDatum[] {
		return buckets.map((b) => ({ label: b.label, value: b.value, color: b.color }));
	}

	let totalClientGroups = $derived(stats?.clients?.length ?? 0);
	let totalMcpGroups = $derived(stats?.mcp_servers?.length ?? 0);
	let totalSkillGroups = $derived(stats?.skills?.length ?? 0);

	type TimelineRow = { scanned_at: string; category: 'scans' };

	let timelineRows = $derived<TimelineRow[]>(
		(stats?.scan_timestamps ?? []).map((ts) => ({
			scanned_at: ts,
			category: 'scans' as const
		}))
	);

	let totalScansInWindow = $derived(stats?.scan_timestamps?.length ?? 0);

	async function reload() {
		loading = true;
		try {
			stats = await AdminService.getDeviceScanStats({ start: range.start, end: range.end });
		} finally {
			loading = false;
		}
	}

	function syncUrl() {
		const next = new URL(page.url);
		const defaultStart = Date.now() - DEFAULT_WINDOW_MS;
		const startMs = new Date(range.start).getTime();
		const endMs = new Date(range.end).getTime();
		if (Math.abs(startMs - defaultStart) > 60_000 || Math.abs(endMs - Date.now()) > 60_000) {
			next.searchParams.set('start', range.start);
			next.searchParams.set('end', range.end);
		} else {
			next.searchParams.delete('start');
			next.searchParams.delete('end');
		}
		replaceState(next, {});
	}

	function onRangeChange({ start, end }: { start: Date | string; end: Date | string }) {
		range = {
			start: new Date(start).toISOString(),
			end: new Date(end).toISOString()
		};
		syncUrl();
		reload();
	}

	type SeeMoreTarget = 'devices' | 'mcps' | 'skills';

	type StatTile = {
		key: string;
		label: string;
		value: number;
		icon: typeof Laptop;
		seeMore?: SeeMoreTarget;
	};

	let tiles = $derived<StatTile[]>([
		{
			key: 'devices',
			label: 'Unique Devices',
			value: stats?.device_count ?? 0,
			icon: Laptop,
			seeMore: 'devices'
		},
		{
			key: 'users',
			label: 'Unique Users',
			value: stats?.user_count ?? 0,
			icon: Users
		},
		{
			key: 'clients',
			label: 'Unique Clients',
			value: totalClientGroups,
			icon: MonitorCheck
		},
		{
			key: 'mcps',
			label: 'Unique MCPs',
			value: totalMcpGroups,
			icon: Server,
			seeMore: 'mcps'
		},
		{
			key: 'skills',
			label: 'Unique Skills',
			value: totalSkillGroups,
			icon: PencilRuler,
			seeMore: 'skills'
		}
	]);

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | Device Dashboard</title>
</svelte:head>

<Layout title="Dashboard">
	<div
		class="flex h-full w-full flex-col gap-4"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		<div class="flex flex-wrap items-center gap-2">
			<AuditLogCalendar
				start={new Date(range.start)}
				end={new Date(range.end)}
				onChange={onRangeChange}
				disabled={loading}
			/>
		</div>

		{#if !stats || stats.device_count === 0}
			<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
				<ScanLine class="text-on-surface1 size-24 opacity-50" />
				<h4 class="text-on-surface1 text-lg font-semibold">No device scans in this window</h4>
				<p class="text-on-surface1 text-sm font-light">
					Adjust the date range or run <code class="font-mono">obot scan</code> from a managed device.
				</p>
			</div>
		{:else}
			<div
				class="paper dark:divide-surface3 divide-surface2 grid grid-cols-2 divide-x sm:grid-cols-3 lg:grid-cols-5"
			>
				{#each tiles as tile (tile.key)}
					{@render statCell(tile)}
				{/each}
			</div>

			<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
				{@render donutCard('Clients', clientBuckets, totalClientGroups, 'No clients observed yet.')}
				{@render donutCard('Top MCPs', mcpBuckets, totalMcpGroups, 'No MCP servers observed yet.')}
				{@render donutCard('Top Skills', skillBuckets, totalSkillGroups, 'No skills observed yet.')}
				{@render timelineCard()}
			</div>
		{/if}
	</div>
</Layout>

{#snippet statCell(tile: StatTile)}
	{#if tile.seeMore === 'devices'}
		<a
			href={resolve('/admin/devices')}
			onclick={(e) => {
				e.preventDefault();
				openUrl(resolve('/admin/devices'), e.ctrlKey || e.metaKey);
			}}
			class="hover:bg-surface2/50 group flex items-center justify-between gap-3 p-4 transition-colors"
		>
			{@render statCellInner(tile, true)}
		</a>
	{:else if tile.seeMore === 'mcps'}
		<a
			href={resolve('/admin/device-mcp-servers')}
			onclick={(e) => {
				e.preventDefault();
				openUrl(resolve('/admin/device-mcp-servers'), e.ctrlKey || e.metaKey);
			}}
			class="hover:bg-surface2/50 group flex items-center justify-between gap-3 p-4 transition-colors"
		>
			{@render statCellInner(tile, true)}
		</a>
	{:else if tile.seeMore === 'skills'}
		<a
			href={resolve('/admin/device-skills')}
			onclick={(e) => {
				e.preventDefault();
				openUrl(resolve('/admin/device-skills'), e.ctrlKey || e.metaKey);
			}}
			class="hover:bg-surface2/50 group flex items-center justify-between gap-3 p-4 transition-colors"
		>
			{@render statCellInner(tile, true)}
		</a>
	{:else}
		<div class="flex items-center justify-between gap-3 p-4">
			{@render statCellInner(tile, false)}
		</div>
	{/if}
{/snippet}

{#snippet statCellInner(tile: StatTile, clickable: boolean)}
	<div class="flex min-w-0 flex-col">
		<span class="text-on-surface1 truncate text-xs">
			{tile.label}{#if clickable}<ChevronRight
					class="ml-0.5 inline size-3 -translate-x-0.5 opacity-0 transition group-hover:translate-x-0 group-hover:opacity-100"
				/>{/if}
		</span>
		<span class="text-2xl font-semibold tabular-nums">{tile.value}</span>
	</div>
	<tile.icon class="text-primary size-7 shrink-0" />
{/snippet}

{#snippet donutCard(title: string, buckets: TopBucket[], totalGroups: number, emptyMsg: string)}
	<div class="paper flex h-full flex-col gap-2">
		<div class="flex items-baseline justify-between gap-2">
			<h4 class="font-semibold">{title}</h4>
			{#if totalGroups > TOP_N}
				<span class="text-on-surface1 text-xs">
					top {TOP_N} of {totalGroups}
				</span>
			{:else if totalGroups > 0}
				<span class="text-on-surface1 text-xs">
					{totalGroups}
					{totalGroups === 1 ? 'entry' : 'entries'}
				</span>
			{/if}
		</div>

		{#if buckets.length === 0}
			<p class="text-on-surface1 py-8 text-center text-sm font-light">{emptyMsg}</p>
		{:else}
			<div class="flex flex-col items-center gap-4 sm:flex-row">
				<div class="size-56 shrink-0">
					<DonutGraph class="h-56 w-56" data={bucketsToDonut(buckets)} hideLegend />
				</div>
				<ul class="flex w-full flex-col gap-1 text-sm">
					{#each buckets as bucket (bucket.key)}
						{#if bucket.drilldown === 'mcp' && !bucket.isOther}
							<li>
								<a
									href={resolve(`/admin/device-mcp-servers/${encodeURIComponent(bucket.key)}`)}
									onclick={(e) => {
										e.preventDefault();
										openUrl(
											resolve(`/admin/device-mcp-servers/${encodeURIComponent(bucket.key)}`),
											e.ctrlKey || e.metaKey
										);
									}}
									class="hover:bg-surface1 dark:hover:bg-surface2 group -mx-1 flex items-center gap-2 rounded-md px-1 py-0.5 transition-colors"
								>
									<span
										class="size-3 shrink-0 rounded-full"
										style="background-color: {bucket.color}"
									></span>
									<span class="flex-1 truncate">{bucket.label}</span>
									<span class="text-on-surface1 tabular-nums">{bucket.value}</span>
									<ChevronRight class="text-on-surface1 size-3 opacity-0 group-hover:opacity-100" />
								</a>
							</li>
						{:else if bucket.drilldown === 'skill' && !bucket.isOther}
							<li>
								<a
									href={resolve(`/admin/device-skills/${encodeURIComponent(bucket.key)}`)}
									onclick={(e) => {
										e.preventDefault();
										openUrl(
											resolve(`/admin/device-skills/${encodeURIComponent(bucket.key)}`),
											e.ctrlKey || e.metaKey
										);
									}}
									class="hover:bg-surface1 dark:hover:bg-surface2 group -mx-1 flex items-center gap-2 rounded-md px-1 py-0.5 transition-colors"
								>
									<span
										class="size-3 shrink-0 rounded-full"
										style="background-color: {bucket.color}"
									></span>
									<span class="flex-1 truncate">{bucket.label}</span>
									<span class="text-on-surface1 tabular-nums">{bucket.value}</span>
									<ChevronRight class="text-on-surface1 size-3 opacity-0 group-hover:opacity-100" />
								</a>
							</li>
						{:else}
							{@render legendStatic(bucket)}
						{/if}
					{/each}
				</ul>
			</div>
		{/if}
	</div>
{/snippet}

{#snippet timelineCard()}
	<div class="paper flex h-full flex-col gap-2">
		<div class="flex items-baseline justify-between gap-2">
			<h4 class="font-semibold">Scan Timeline</h4>
			{#if totalScansInWindow > 0}
				<span class="text-on-surface1 text-xs">
					{totalScansInWindow}
					{totalScansInWindow === 1 ? 'submission' : 'submissions'}
				</span>
			{/if}
		</div>

		{#if timelineRows.length === 0}
			<p
				class="text-on-surface1 flex min-h-56 flex-1 items-center justify-center text-sm font-light"
			>
				No scan submissions in this window.
			</p>
		{:else}
			<div class="text-on-surface1 flex min-h-56 flex-1 items-center justify-center">
				<StackedTimeline
					start={new Date(range.start)}
					end={new Date(range.end)}
					data={timelineRows}
					categoryKey="category"
					dateKey="scanned_at"
					legend={{ hideCategoryLabel: true }}
				/>
			</div>
		{/if}
	</div>
{/snippet}

{#snippet legendStatic(bucket: TopBucket)}
	<li
		class="-mx-1 flex items-center gap-2 rounded-md px-1 py-0.5"
		class:text-on-surface1={bucket.isOther}
	>
		<span class="size-3 shrink-0 rounded-full" style="background-color: {bucket.color}"></span>
		<span class="flex-1 truncate" class:italic={bucket.isOther}>
			{bucket.label}
			{#if bucket.isOther && bucket.otherCount !== undefined}
				<span class="text-on-surface1 ml-1 not-italic">({bucket.otherCount} more)</span>
			{/if}
		</span>
		<span class="text-on-surface1 tabular-nums">{bucket.value}</span>
	</li>
{/snippet}
