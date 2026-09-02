<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import DeviceScanDonutCard from '$lib/components/admin/device-scan/DeviceScanDonutCard.svelte';
	import DeviceScanTimelineCard from '$lib/components/admin/device-scan/DeviceScanTimelineCard.svelte';
	import { buildDeviceScanTopBuckets } from '$lib/components/admin/device-scan/deviceScanTopBuckets';
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
	} from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';

	interface Props {
		stats?: DeviceScanStats | null;
		range?: { start: string; end: string };
	}

	let { stats: initialStats = null, range: initialRange }: Props = $props();

	function rangeFromUrl() {
		return {
			start:
				page.url.searchParams.get('start') ??
				initialRange?.start ??
				new Date(Date.now() - DEFAULT_WINDOW_MS).toISOString(),
			end: page.url.searchParams.get('end') ?? initialRange?.end ?? new Date().toISOString()
		};
	}

	let stats = $state<DeviceScanStats | null>(untrack(() => initialStats ?? null));
	let range = $state<{ start: string; end: string }>(untrack(() => rangeFromUrl()));
	let loading = $state(false);

	onMount(() => {
		if (range.start !== initialRange?.start || range.end !== initialRange?.end) {
			reload();
		}
	});

	let clientBuckets = $derived(
		buildDeviceScanTopBuckets<DeviceClientStat>(
			stats?.clients,
			(c) => c.name,
			(c) => c.name,
			(c) => c.deviceCount
		)
	);

	let mcpBuckets = $derived(
		buildDeviceScanTopBuckets<DeviceMCPServerStat>(
			stats?.mcpServers,
			(m) => m.configHash,
			(m) => m.name?.trim() || '(unnamed)',
			(m) => m.deviceCount,
			'mcp'
		)
	);

	let skillBuckets = $derived(
		buildDeviceScanTopBuckets<DeviceSkillStat>(
			stats?.skills,
			(s) => s.name,
			(s) => s.name,
			(s) => s.deviceCount,
			'skill'
		)
	);

	let totalClientGroups = $derived(stats?.clients?.length ?? 0);
	let totalMcpGroups = $derived(stats?.mcpServers?.length ?? 0);
	let totalSkillGroups = $derived(stats?.skills?.length ?? 0);

	type TimelineRow = { scanned_at: string; category: 'scans' };

	let timelineRows = $derived<TimelineRow[]>(
		(stats?.scanTimestamps ?? []).map((ts) => ({
			scanned_at: ts,
			category: 'scans' as const
		}))
	);

	let totalScansInWindow = $derived(stats?.scanTimestamps?.length ?? 0);

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

	type StatTile = {
		key: string;
		label: string;
		value: number;
		icon: typeof Laptop;
		seeMore?: `/${string}`;
	};

	let tiles = $derived<StatTile[]>([
		{
			key: 'devices',
			label: 'Unique Devices',
			value: stats?.deviceCount ?? 0,
			icon: Laptop,
			seeMore: '/inventory?view=devices'
		},
		{
			key: 'users',
			label: 'Unique Users',
			value: stats?.userCount ?? 0,
			icon: Users
		},
		{
			key: 'clients',
			label: 'Unique Clients',
			value: totalClientGroups,
			icon: MonitorCheck,
			seeMore: '/inventory?view=device-clients'
		},
		{
			key: 'mcps',
			label: 'Unique MCPs',
			value: totalMcpGroups,
			icon: Server,
			seeMore: '/inventory?view=device-mcp-servers'
		},
		{
			key: 'skills',
			label: 'Unique Skills',
			value: totalSkillGroups,
			icon: PencilRuler,
			seeMore: '/inventory?view=device-skills'
		}
	]);
</script>

<div class="flex flex-wrap items-center gap-2">
	<AuditLogCalendar
		start={new Date(range.start)}
		end={new Date(range.end)}
		onChange={onRangeChange}
		disabled={loading}
	/>
</div>

{#if !stats || stats.deviceCount === 0}
	<div class="mx-auto mt-12 flex w-md flex-col items-center gap-4 text-center">
		<ScanLine class="text-muted-content size-24 opacity-50" />
		<h4 class="text-muted-content text-lg font-semibold">No device scans in this window</h4>
		<p class="text-muted-content text-sm font-light">
			Adjust the date range or run <code class="font-mono">obot scan</code> from a managed device.
		</p>
	</div>
{:else}
	<div
		class="paper dark:divide-base-400 divide-base-300 grid grid-cols-2 divide-x sm:grid-cols-3 lg:grid-cols-5"
	>
		{#each tiles as tile (tile.key)}
			{@render statCell(tile)}
		{/each}
	</div>

	<div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
		<DeviceScanDonutCard
			title="Clients"
			buckets={clientBuckets}
			totalGroups={totalClientGroups}
			emptyMsg="No clients observed yet."
		/>
		<DeviceScanDonutCard
			title="Top MCPs"
			buckets={mcpBuckets}
			totalGroups={totalMcpGroups}
			emptyMsg="No MCP servers observed yet."
		/>
		<DeviceScanDonutCard
			title="Top Skills"
			buckets={skillBuckets}
			totalGroups={totalSkillGroups}
			emptyMsg="No skills observed yet."
		/>
		<DeviceScanTimelineCard
			rangeStart={range.start}
			rangeEnd={range.end}
			{timelineRows}
			totalSubmissions={totalScansInWindow}
		/>
	</div>
{/if}

{#snippet statCell(tile: StatTile)}
	{#if tile.seeMore}
		<a
			href={resolve(tile.seeMore)}
			onclick={(e) => {
				e.preventDefault();
				openUrl(resolve(tile.seeMore!), e.ctrlKey || e.metaKey);
			}}
			class="hover:bg-base-300/50 group flex items-center justify-between gap-3 p-4 transition-colors"
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
		<span class="text-muted-content truncate text-xs">
			{tile.label}{#if clickable}<ChevronRight
					class="ml-0.5 inline size-3 -translate-x-0.5 opacity-0 transition group-hover:translate-x-0 group-hover:opacity-100"
				/>{/if}
		</span>
		<span class="text-2xl font-semibold tabular-nums">{tile.value}</span>
	</div>
	<tile.icon class="text-primary size-7 shrink-0" />
{/snippet}
