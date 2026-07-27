<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import OverflowContainer from '$lib/components/OverflowContainer.svelte';
	import AuditLogCalendar from '$lib/components/admin/audit-logs/AuditLogCalendar.svelte';
	import DeviceScanDonutCard from '$lib/components/admin/device-scan/DeviceScanDonutCard.svelte';
	import DeviceScanTimelineCard from '$lib/components/admin/device-scan/DeviceScanTimelineCard.svelte';
	import { buildDeviceScanTopBuckets } from '$lib/components/admin/device-scan/deviceScanTopBuckets';
	import Devices from '$lib/components/admin/devices/Devices.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		AdminService,
		type DeviceClientStat,
		type DeviceMCPServerStat,
		type DeviceScanStats,
		type DeviceSkillStat
	} from '$lib/services';
	import { clearUrlParams, goto, replaceState } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import Configuration from './Configuration.svelte';
	import DeviceClients from './DeviceClients.svelte';
	import DeviceMcpServers from './DeviceMcpServers.svelte';
	import DeviceSkills from './DeviceSkills.svelte';
	import { DEFAULT_WINDOW_MS } from './constants';
	import {
		ChevronLeft,
		ChevronRight,
		Laptop,
		MonitorCheck,
		PencilRuler,
		ScanLine,
		Server,
		Users
	} from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();

	type View =
		| 'configuration'
		| 'overview'
		| 'devices'
		| 'device-clients'
		| 'device-mcp-servers'
		| 'device-skills';
	const views: { label: string; value: View }[] = [
		{
			label: 'Configuration',
			value: 'configuration' as const
		},
		{
			label: 'Overview',
			value: 'overview' as const
		},
		{
			label: 'Devices',
			value: 'devices' as const
		},
		{
			label: 'Device Clients',
			value: 'device-clients' as const
		},
		{
			label: 'Device MCP Servers',
			value: 'device-mcp-servers' as const
		},
		{
			label: 'Device Skills',
			value: 'device-skills' as const
		}
	];

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

	const defaultView: View = untrack(() => (data.configuration ? 'overview' : 'configuration'));
	let view = $derived.by<View>(() => {
		const requested = page.url.searchParams.get('view') as View | null;
		return requested && views.some((candidate) => candidate.value === requested)
			? requested
			: defaultView;
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
			value: stats?.deviceCount ?? 0,
			icon: Laptop,
			seeMore: 'devices'
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
	<title>Obot | Devices</title>
</svelte:head>

<Layout title="Devices" classes={{ container: 'justify-start' }}>
	<div
		class="flex h-full w-full flex-col gap-4"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		<div class="w-full">
			<OverflowContainer
				class="scrollbar-none flex shrink-0 min-h-12 w-full items-center gap-2 overflow-x-auto"
				style="scroll-behavior: smooth;"
			>
				{#snippet children({ x, hasMoreLeft, hasMoreRight, scrollLeft, scrollRight })}
					{#if x}
						<button
							disabled={!hasMoreLeft}
							onclick={scrollLeft}
							class="shrink-0 z-20 bg-base-200 dark:bg-base-100 sticky left-0 flex aspect-square h-full items-center justify-center rounded-l-md p-2.5 opacity-100 transition-all duration-200 disabled:opacity-30"
						>
							<ChevronLeft class="size-full" />
						</button>
					{/if}

					<div class="flex flex-1 flex-col">
						<div class="flex flex-1 relative z-10">
							{#each views as viewOption (viewOption.value)}
								<button
									class={twMerge(
										'border-b-2 text-nowrap border-transparent px-8 py-2 transition-colors duration-400',
										view === viewOption.value
											? 'border-primary'
											: 'hover:border-primary/25 text-muted-content hover:text-base-content'
									)}
									onclick={() => {
										clearUrlParams(
											Array.from(page.url.searchParams.keys()).filter((key) => key !== 'view')
										);
										goto(`/admin/devices?view=${viewOption.value}`);
									}}
								>
									{viewOption.label}
								</button>
							{/each}
						</div>
						<div class="bg-base-400 h-0.5 w-full shrink-0 -translate-y-0.5"></div>
					</div>

					{#if x}
						<button
							disabled={!hasMoreRight}
							onclick={scrollRight}
							class="shrink-0 z-20 bg-base-200 dark:bg-base-100 sticky right-0 flex aspect-square h-full items-center justify-center rounded-r-md p-2.5 opacity-100 transition-all duration-200 disabled:opacity-30"
						>
							<ChevronRight class="size-full" />
						</button>
					{/if}
				{/snippet}
			</OverflowContainer>
		</div>

		{#if view === 'configuration'}
			<Configuration
				configuration={data.configuration}
				enrollmentKeys={data.enrollmentKeys}
				assetSource={data.assetSource}
				assets={data.assets}
				assetLoadError={data.assetLoadError}
			/>
		{:else if view === 'overview'}
			{@render overview()}
		{:else if view === 'devices'}
			<Devices />
		{:else if view === 'device-clients'}
			<DeviceClients />
		{:else if view === 'device-mcp-servers'}
			<DeviceMcpServers />
		{:else if view === 'device-skills'}
			<DeviceSkills />
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
			class="hover:bg-base-300/50 group flex items-center justify-between gap-3 p-4 transition-colors"
		>
			{@render statCellInner(tile, true)}
		</a>
	{:else if tile.seeMore === 'mcps'}
		<a
			href={resolve('/admin/devices/mcp-servers')}
			onclick={(e) => {
				e.preventDefault();
				openUrl(resolve('/admin/devices/mcp-servers'), e.ctrlKey || e.metaKey);
			}}
			class="hover:bg-base-300/50 group flex items-center justify-between gap-3 p-4 transition-colors"
		>
			{@render statCellInner(tile, true)}
		</a>
	{:else if tile.seeMore === 'skills'}
		<a
			href={resolve('/admin/devices/skills')}
			onclick={(e) => {
				e.preventDefault();
				openUrl(resolve('/admin/devices/skills'), e.ctrlKey || e.metaKey);
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

{#snippet overview()}
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
{/snippet}
