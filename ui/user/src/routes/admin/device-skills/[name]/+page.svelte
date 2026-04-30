<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		AdminService,
		type DeviceSkillOccurrence,
		type DeviceSkillOccurrenceResponse,
		type DeviceSkillDetail
	} from '$lib/services';
	import { formatTimeAgo } from '$lib/time';
	import { goto, setFilterUrlParams } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { FileText } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const PAGE_SIZE = untrack(() => data?.pageSize ?? 50);

	let detail = $derived<DeviceSkillDetail | null | undefined>(data?.detail);
	let occurrencesResp = $state<DeviceSkillOccurrenceResponse>(
		untrack(() => data?.occurrences ?? { items: [], total: 0, limit: PAGE_SIZE, offset: 0 })
	);
	let pageIndex = $derived(
		Math.floor(Number(page.url.searchParams.get('offset') ?? 0) / PAGE_SIZE)
	);
	let loading = $state(false);

	let skillName = $derived(page.params.name ?? '');

	type Row = DeviceSkillOccurrence & {
		shortDeviceID: string;
		scannedRelative: string;
	};

	let rows = $derived<Row[]>(
		(occurrencesResp.items ?? []).map((o, i) => ({
			...o,
			rowIndex: ((occurrencesResp.offset ?? 0) + i + 1).toString(),
			shortDeviceID: (o.deviceID ?? '').slice(0, 12),
			scannedRelative: formatTimeAgo(o.scannedAt).relativeTime
		}))
	);

	let total = $derived(occurrencesResp.total ?? 0);
	let lastPageIndex = $derived(total > 0 ? Math.ceil(total / PAGE_SIZE) - 1 : 0);

	async function fetchPage(idx: number) {
		if (!skillName) return;
		loading = true;
		try {
			occurrencesResp = await AdminService.listDeviceSkillOccurrences(skillName, {
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
	<title>Obot | Skill</title>
</svelte:head>

<Layout
	title="Skill"
	showBackButton
	onBackButtonClick={() => {
		if (typeof window !== 'undefined' && window.history.length > 1) {
			window.history.back();
		} else {
			goto(resolve('/admin/device-skills'));
		}
	}}
>
	<div
		class="flex flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !detail}
			<p class="text-muted-content text-sm font-light">Skill not found.</p>
		{:else}
			<div class="dark:bg-base-300 bg-base-100 flex flex-col gap-4 rounded-md p-4 shadow-sm">
				<div class="flex flex-col gap-2">
					<h2 class="flex items-center gap-2 text-xl font-semibold">
						{detail.name}
						{#if detail.hasScripts}
							<span class="pill-primary bg-primary text-xs">has scripts</span>
						{/if}
					</h2>
					<div class="text-muted-content flex flex-wrap items-center gap-3 text-xs">
						<span>{detail.deviceCount} device{detail.deviceCount === 1 ? '' : 's'}</span>
						<span>·</span>
						<span>{detail.userCount} user{detail.userCount === 1 ? '' : 's'}</span>
						<span>·</span>
						<span>
							{detail.observationCount} observation{detail.observationCount === 1 ? '' : 's'}
						</span>
					</div>
				</div>

				{#if detail.description}
					<div class="flex flex-col gap-1">
						<span class="text-muted-content text-xs uppercase">Description</span>
						<p class="text-sm">{detail.description}</p>
					</div>
				{/if}

				<div class="grid grid-cols-1 gap-3 md:grid-cols-2">
					{#if detail.gitRemoteURL}
						<div class="flex flex-col gap-1">
							<span class="text-muted-content text-xs uppercase">Git remote</span>
							<code class="font-mono text-xs break-all">{detail.gitRemoteURL}</code>
						</div>
					{/if}
					{#if detail.files?.length}
						<div class="flex flex-col gap-1 md:col-span-2">
							<span class="text-muted-content text-xs uppercase">
								Files ({detail.files.length})
							</span>
							<ul class="flex flex-col gap-0.5">
								{#each detail.files as f (f)}
									<li class="flex items-center gap-1.5 font-mono text-xs">
										<FileText class="text-muted-content size-3 shrink-0" />
										<span class="break-all">{f}</span>
									</li>
								{/each}
							</ul>
						</div>
					{/if}
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<h3 class="text-muted-content text-sm font-semibold">
					Occurrences · {total}
				</h3>
				<Table
					data={rows}
					fields={[
						'rowIndex',
						'shortDeviceID',
						'scannedRelative',
						'client',
						'scope',
						'projectPath'
					]}
					headers={[
						{ title: '#', property: 'rowIndex' },
						{ title: 'Device', property: 'shortDeviceID' },
						{ title: 'Scanned', property: 'scannedRelative' },
						{ title: 'Client', property: 'client' },
						{ title: 'Scope', property: 'scope' },
						{ title: 'Project', property: 'projectPath' }
					]}
					onClickRow={(d, isCtrlClick) => {
						openUrl(
							resolve(`/admin/devices/${d.deviceID}/scans/${d.deviceScanID}/skills/${d.id}`),
							isCtrlClick
						);
					}}
				>
					{#snippet onRenderColumn(property, d: Row)}
						{#if property === 'shortDeviceID'}
							<a
								href={resolve(`/admin/devices/${d.deviceID}`)}
								class="font-mono text-xs btn-link text-primary"
								title={d.deviceID}
								onclick={(e) => e.stopPropagation()}
							>
								{d.shortDeviceID}
							</a>
						{:else if property === 'projectPath'}
							<span class="text-muted-content font-mono text-xs">{d.projectPath ?? '—'}</span>
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
