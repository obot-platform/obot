<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import {
		AdminService,
		type DeviceSkillDetail,
		type DeviceSkillOccurrence,
		type DeviceSkillOccurrenceList
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
	let occurrencesResp = $state<DeviceSkillOccurrenceList>(
		untrack(() => data?.occurrences ?? { items: [], total: 0, limit: PAGE_SIZE, offset: 0 })
	);
	let pageIndex = $derived(
		Math.floor(Number(page.url.searchParams.get('offset') ?? 0) / PAGE_SIZE)
	);
	let loading = $state(false);

	let skillName = $derived(page.params.name ?? '');

	type Row = DeviceSkillOccurrence & {
		id: string;
		short_device_id: string;
		scanned_relative: string;
	};

	let rows = $derived<Row[]>(
		(occurrencesResp.items ?? []).map((o, i) => ({
			...o,
			id: `${o.device_scan_id}-${o.index}-${i}`,
			short_device_id: (o.device_id ?? '').slice(0, 12),
			scanned_relative: formatTimeAgo(o.scanned_at).relativeTime
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
			<p class="text-on-surface1 text-sm font-light">Skill not found.</p>
		{:else}
			<div class="dark:bg-surface2 bg-background flex flex-col gap-4 rounded-md p-4 shadow-sm">
				<div class="flex flex-col gap-2">
					<h2 class="flex items-center gap-2 text-xl font-semibold">
						{detail.name}
						{#if detail.has_scripts}
							<span class="pill-primary bg-primary text-xs">has scripts</span>
						{/if}
					</h2>
					<div class="text-on-surface1 flex flex-wrap items-center gap-3 text-xs">
						<span>{detail.device_count} device{detail.device_count === 1 ? '' : 's'}</span>
						<span>·</span>
						<span>{detail.user_count} user{detail.user_count === 1 ? '' : 's'}</span>
						<span>·</span>
						<span>
							{detail.observation_count} observation{detail.observation_count === 1 ? '' : 's'}
						</span>
					</div>
				</div>

				{#if detail.description}
					<div class="flex flex-col gap-1">
						<span class="text-on-surface2 text-xs uppercase">Description</span>
						<p class="text-sm">{detail.description}</p>
					</div>
				{/if}

				<div class="grid grid-cols-1 gap-3 md:grid-cols-2">
					{#if detail.git_remote_url}
						<div class="flex flex-col gap-1">
							<span class="text-on-surface2 text-xs uppercase">Git remote</span>
							<code class="font-mono text-xs break-all">{detail.git_remote_url}</code>
						</div>
					{/if}
					{#if detail.files?.length}
						<div class="flex flex-col gap-1 md:col-span-2">
							<span class="text-on-surface2 text-xs uppercase">
								Files ({detail.files.length})
							</span>
							<ul class="flex flex-col gap-0.5">
								{#each detail.files as f (f)}
									<li class="flex items-center gap-1.5 font-mono text-xs">
										<FileText class="text-on-surface1 size-3 shrink-0" />
										<span class="break-all">{f}</span>
									</li>
								{/each}
							</ul>
						</div>
					{/if}
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<h3 class="text-on-surface1 text-sm font-semibold">
					Devices · {total} occurrence{total === 1 ? '' : 's'}
				</h3>
				<Table
					data={rows}
					fields={['short_device_id', 'scanned_relative', 'client', 'scope', 'project_path']}
					headers={[
						{ title: 'Device', property: 'short_device_id' },
						{ title: 'Scanned', property: 'scanned_relative' },
						{ title: 'Client', property: 'client' },
						{ title: 'Scope', property: 'scope' },
						{ title: 'Project', property: 'project_path' }
					]}
					onClickRow={(d, isCtrlClick) => {
						openUrl(
							resolve(`/admin/devices/${d.device_id}/scans/${d.device_scan_id}/skills/${d.index}`),
							isCtrlClick
						);
					}}
				>
					{#snippet onRenderColumn(property, d: Row)}
						{#if property === 'short_device_id'}
							<span class="font-mono text-xs" title={d.device_id}>{d.short_device_id}</span>
						{:else if property === 'project_path'}
							<span class="text-on-surface1 font-mono text-xs">{d.project_path ?? '—'}</span>
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
