<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		AdminService,
		type DeviceMCPServerOccurrence,
		type DeviceMCPServerOccurrenceResponse,
		type DeviceMCPServerDetail
	} from '$lib/services';
	import { formatTimeAgo } from '$lib/time';
	import { goto, setFilterUrlParams } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const PAGE_SIZE = untrack(() => data?.pageSize ?? 50);

	let detail = $derived<DeviceMCPServerDetail | null | undefined>(data?.detail);
	let occurrencesResp = $state<DeviceMCPServerOccurrenceResponse>(
		untrack(() => data?.occurrences ?? { items: [], total: 0, limit: PAGE_SIZE, offset: 0 })
	);
	let pageIndex = $derived(
		Math.floor(Number(page.url.searchParams.get('offset') ?? 0) / PAGE_SIZE)
	);
	let loading = $state(false);

	let configHash = $derived(page.params.hash);

	type Row = DeviceMCPServerOccurrence & {
		shortDeviceID: string;
		scannedRelative: string;
	};

	let rows = $derived<Row[]>(
		(occurrencesResp.items ?? []).map((o, i) => ({
			...o,
			rowIndex: (i + 1).toString(),
			shortDeviceID: (o.deviceID ?? '').slice(0, 12),
			scannedRelative: formatTimeAgo(o.scannedAt).relativeTime
		}))
	);

	let total = $derived(occurrencesResp.total ?? 0);
	let lastPageIndex = $derived(total > 0 ? Math.ceil(total / PAGE_SIZE) - 1 : 0);

	async function fetchPage(idx: number) {
		if (!configHash) return;
		loading = true;
		try {
			occurrencesResp = await AdminService.listDeviceMCPServerOccurrences(configHash, {
				limit: PAGE_SIZE,
				offset: idx * PAGE_SIZE
			});
			setFilterUrlParams('offset', idx > 0 ? [String(idx * PAGE_SIZE)] : []);
		} finally {
			loading = false;
		}
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<svelte:head>
	<title>Obot | MCP Server</title>
</svelte:head>

<Layout
	title="MCP Server"
	showBackButton
	onBackButtonClick={() => {
		if (typeof window !== 'undefined' && window.history.length > 1) {
			window.history.back();
		} else {
			goto('/admin/device-mcp-servers');
		}
	}}
>
	<div
		class="flex flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !detail}
			<p class="text-on-surface1 text-sm font-light">MCP server not found.</p>
		{:else}
			<div class="dark:bg-surface2 bg-background flex flex-col gap-4 rounded-md p-4 shadow-sm">
				<div class="flex flex-col gap-2">
					<h2 class="flex items-center gap-2 text-xl font-semibold">
						{#if detail.name?.trim()}
							{detail.name}
						{:else}
							<span class="text-on-surface2 italic">(unnamed)</span>
						{/if}
						<span class="pill-primary bg-primary text-xs">{detail.transport}</span>
					</h2>
					<div class="text-on-surface1 flex flex-wrap items-center gap-3 text-xs">
						<span>{detail.deviceCount} device{detail.deviceCount === 1 ? '' : 's'}</span>
						<span>·</span>
						<span>{detail.userCount} user{detail.userCount === 1 ? '' : 's'}</span>
						<span>·</span>
						<span>{detail.clientCount} client{detail.clientCount === 1 ? '' : 's'}</span>
					</div>
				</div>

				<div class="grid grid-cols-1 gap-3 md:grid-cols-2">
					{#if detail.command}
						<div class="flex flex-col gap-1">
							<span class="text-on-surface2 text-xs uppercase">Command</span>
							<code class="font-mono text-xs break-all">
								{[detail.command, ...(detail.args ?? [])].join(' ')}
							</code>
						</div>
					{/if}
					{#if detail.url}
						<div class="flex flex-col gap-1">
							<span class="text-on-surface2 text-xs uppercase">URL</span>
							<code class="font-mono text-xs break-all">{detail.url}</code>
						</div>
					{/if}
					{#if detail.envKeys?.length}
						<div class="flex flex-col gap-1">
							<span class="text-on-surface2 text-xs uppercase">Env keys</span>
							<div class="flex flex-wrap gap-1">
								{#each detail.envKeys as k (k)}
									<code class="bg-surface3 rounded px-1.5 py-0.5 font-mono text-xs">{k}</code>
								{/each}
							</div>
						</div>
					{/if}
					{#if detail.headerKeys?.length}
						<div class="flex flex-col gap-1">
							<span class="text-on-surface2 text-xs uppercase">Header keys</span>
							<div class="flex flex-wrap gap-1">
								{#each detail.headerKeys as k (k)}
									<code class="bg-surface3 rounded px-1.5 py-0.5 font-mono text-xs">{k}</code>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<h3 class="text-on-surface1 text-sm font-semibold">
					Occurrences · {total}
				</h3>
				<Table
					data={rows}
					fields={['rowIndex', 'shortDeviceID', 'scannedRelative', 'client', 'scope']}
					headers={[
						{ title: '#', property: 'rowIndex' },
						{ title: 'Device', property: 'shortDeviceID' },
						{ title: 'Scanned', property: 'scannedRelative' },
						{ title: 'Client', property: 'client' },
						{ title: 'Scope', property: 'scope' }
					]}
					onClickRow={(d, isCtrlClick) => {
						openUrl(
							`/admin/devices/${d.deviceID}/scans/${d.deviceScanID}/mcp/${d.id}`,
							isCtrlClick
						);
					}}
				>
					{#snippet onRenderColumn(property, d: Row)}
						{#if property === 'shortDeviceID'}
							<a
								href={resolve(`/admin/devices/${d.deviceID}`)}
								class="font-mono text-xs btn-link text-blue-500"
								title={d.deviceID}
								onclick={(e) => e.stopPropagation()}
							>
								{d.shortDeviceID}
							</a>
						{:else}
							{d[property as keyof Row]}
						{/if}
					{/snippet}
				</Table>

				{#if total > PAGE_SIZE}
					<Pagination {pageIndex} {lastPageIndex} {total} {loading} onPageChange={fetchPage} />
				{/if}
			</div>
		{/if}
	</div>
</Layout>
