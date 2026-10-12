<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { deriveDeviceScope, formatDeviceClient } from '$lib/format.js';
	import { m } from '$lib/i18n';
	import {
		AdminService,
		type DeviceSkillOccurrence,
		type DeviceSkillOccurrenceResponse,
		type DeviceSkillDetail
	} from '$lib/services';
	import { formatTimeAgo } from '$lib/time';
	import { goto, setFilterUrlParams } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { FileText } from '@lucide/svelte';
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
			scannedRelative: formatTimeAgo(o.scannedAt).relativeTime,
			scope: o.projectPath ? deriveDeviceScope(o.projectPath) : o.scope
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
	<title>{m.inventory_enforcement_device_skills_page_title_skill()}</title>
</svelte:head>

<Layout
	title={m.inventory_enforcement_skill()}
	showBackButton
	onBackButtonClick={() => {
		if (typeof window !== 'undefined' && window.history.length > 1) {
			window.history.back();
		} else {
			goto(resolve('/inventory?view=device-skills'));
		}
	}}
>
	<div
		class="flex flex-col gap-6"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if !detail}
			<p class="text-muted-content text-sm font-light">
				{m.inventory_enforcement_device_skills_skill_not_found()}
			</p>
		{:else}
			<div class="dark:bg-base-300 bg-base-100 flex flex-col gap-4 rounded-md p-4 shadow-sm">
				<div class="flex flex-col gap-2">
					<h2 class="flex items-center gap-2 text-xl font-semibold">
						{detail.name}
						{#if detail.hasScripts}
							<span class="pill-primary bg-primary text-xs"
								>{m.inventory_enforcement_device_skills_has_scripts()}</span
							>
						{/if}
					</h2>
					<div class="text-muted-content flex flex-wrap items-center gap-3 text-xs">
						<span
							>{detail.deviceCount === 1
								? m.inventory_enforcement_devices_count_one({ count: detail.deviceCount })
								: m.inventory_enforcement_devices_count_other({ count: detail.deviceCount })}</span
						>
						<span>·</span>
						<span
							>{detail.userCount === 1
								? m.inventory_enforcement_users_count_one({ count: detail.userCount })
								: m.inventory_enforcement_users_count_other({ count: detail.userCount })}</span
						>
						<span>·</span>
						<span>
							{detail.observationCount === 1
								? m.inventory_enforcement_device_skills_observations_count_one({
										count: detail.observationCount
									})
								: m.inventory_enforcement_device_skills_observations_count_other({
										count: detail.observationCount
									})}
						</span>
					</div>
				</div>

				{#if detail.description}
					<div class="flex flex-col gap-1">
						<span class="text-muted-content text-xs uppercase">{m.core_description()}</span>
						<p class="text-sm">{detail.description}</p>
					</div>
				{/if}

				<div class="grid grid-cols-1 gap-3 md:grid-cols-2">
					{#if detail.gitRemoteURL}
						<div class="flex flex-col gap-1">
							<span class="text-muted-content text-xs uppercase"
								>{m.inventory_enforcement_label_git_remote()}</span
							>
							<code class="text-sm break-all">{detail.gitRemoteURL}</code>
						</div>
					{/if}
					{#if detail.files?.length}
						<div class="flex flex-col gap-1 md:col-span-2">
							<span class="text-muted-content text-xs uppercase">
								{m.inventory_enforcement_device_skills_files_count({ count: detail.files.length })}
							</span>
							<ul class="flex flex-col gap-1.5">
								{#each detail.files as f (f)}
									<li class="flex items-center gap-1.5 text-xs">
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
					{m.inventory_enforcement_occurrences({ count: total })}
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
						{ title: m.inventory_enforcement_devices_device_title(), property: 'shortDeviceID' },
						{ title: m.inventory_enforcement_col_scanned(), property: 'scannedRelative' },
						{ title: m.inventory_enforcement_col_client(), property: 'client' },
						{ title: m.inventory_enforcement_col_scope(), property: 'scope' },
						{ title: m.inventory_enforcement_device_skills_col_project(), property: 'projectPath' }
					]}
					onClickRow={(d, isCtrlClick) => {
						openUrl(
							resolve(`/inventory/devices/${d.deviceID}/scans/${d.deviceScanID}/skills/${d.id}`),
							isCtrlClick
						);
					}}
				>
					{#snippet onRenderColumn(property, d: Row)}
						{#if property === 'shortDeviceID'}
							<a
								href={resolve(`/inventory/devices/${d.deviceID}`)}
								class="btn-link text-primary"
								title={d.deviceID}
								onclick={(e) => e.stopPropagation()}
							>
								{d.shortDeviceID}
							</a>
						{:else if property === 'projectPath'}
							{d.projectPath ?? '—'}
						{:else if property === 'client'}
							{formatDeviceClient(d.client, d.projectPath)}
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
