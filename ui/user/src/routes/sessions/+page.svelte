<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import LabelBadges from '$lib/components/supervision/LabelBadges.svelte';
	import RiskPill from '$lib/components/supervision/RiskPill.svelte';
	import SurfaceBadge from '$lib/components/supervision/SurfaceBadge.svelte';
	import Pagination from '$lib/components/table/Pagination.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import {
		CONTENT_LABELS,
		listSessions,
		sessionDisplayName,
		SURFACES,
		surfaceLabel,
		type Session,
		type SessionFilters,
		type Surface
	} from '$lib/services/provenance';
	import { formatTimeAgo } from '$lib/time';
	import { clearUrlParams, goto, resolve, setSortUrlParams } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { Activity } from '@lucide/svelte';

	// The session list: every filter lives in the URL so a view can be shared, and sorting is done
	// by the server (risk or last activity).

	const PAGE_SIZE = 50;
	const RANGES = ['1h', '24h', '7d', '30d', 'all'] as const;
	type Range = (typeof RANGES)[number];

	let sessions = $state<Session[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let error = $state('');
	let generation = 0;

	function param(key: string) {
		return page.url.searchParams.get(key) || '';
	}

	// Filters read from the URL. The sort keys match the table's column properties.
	let filters = $derived.by(() => {
		const sort = param('sort') || 'risk';
		const sortDirection = (param('sortDirection') as 'asc' | 'desc') || 'desc';
		const range = (RANGES as readonly string[]).includes(param('range'))
			? (param('range') as Range)
			: '24h';
		const pageIndex = Math.max(0, Number(param('page')) || 0);
		return {
			userId: param('user_id'),
			agent: param('agent'),
			surface: param('surface') as Surface | '',
			minRisk: param('min_risk') ? Number(param('min_risk')) : undefined,
			label: param('label'),
			open: param('open') === 'true',
			query: param('query'),
			range,
			sort,
			sortDirection,
			pageIndex
		};
	});

	function rangeStart(range: Range): string | undefined {
		const hours = { '1h': 1, '24h': 24, '7d': 24 * 7, '30d': 24 * 30, all: 0 }[range];
		if (!hours) return undefined;
		return new Date(Date.now() - hours * 3_600_000).toISOString();
	}

	function rangeLabel(range: Range) {
		switch (range) {
			case '1h':
				return m.supervision_sessions_range_1h();
			case '24h':
				return m.supervision_sessions_range_24h();
			case '7d':
				return m.supervision_sessions_range_7d();
			case '30d':
				return m.supervision_sessions_range_30d();
			default:
				return m.supervision_sessions_range_all();
		}
	}

	async function load() {
		const gen = ++generation;
		loading = true;
		error = '';
		const f = filters;
		const query: SessionFilters = {
			userId: f.userId || undefined,
			agent: f.agent || undefined,
			surface: f.surface || undefined,
			minRisk: f.minRisk,
			label: f.label || undefined,
			open: f.open ? true : undefined,
			query: f.query || undefined,
			startTime: rangeStart(f.range),
			limit: PAGE_SIZE,
			offset: f.pageIndex * PAGE_SIZE,
			sortBy: f.sort === 'lastActivityAt' ? 'last_activity_at' : 'risk',
			sortOrder: f.sortDirection
		};
		try {
			const resp = await listSessions(query, { dontLogErrors: true });
			if (gen !== generation) return;
			sessions = resp.items ?? [];
			total = resp.total ?? sessions.length;
		} catch (e) {
			if (gen !== generation) return;
			error = parseErrorContent(e).message;
		} finally {
			if (gen === generation) loading = false;
		}
	}

	// Reload whenever a filter in the URL changes.
	$effect(() => {
		void JSON.stringify(filters);
		load();
	});

	function setParams(values: Record<string, string | undefined>) {
		const url = new URL(page.url.toString());
		for (const [key, value] of Object.entries(values)) {
			if (value) url.searchParams.set(key, value);
			else url.searchParams.delete(key);
		}
		// Any filter change goes back to the first page.
		if (!('page' in values)) url.searchParams.delete('page');
		goto(url, { replace: true, reset: false, shallow: true });
	}

	let rows = $derived(
		sessions.map((s) => ({
			id: s.id,
			name: sessionDisplayName(s),
			user: s.email || s.userID || '',
			agent: s.agent || s.actor?.agent || '',
			surfaces: s.state.surfaces ?? [],
			labels: s.state.labels ?? [],
			risk: s.state.risk,
			events: s.state.events,
			startedAt: s.startedAt,
			lastActivityAt: s.lastActivityAt,
			open: !s.endedAt,
			killed: !!s.state.killed
		}))
	);

	let hasFilters = $derived(
		!!(
			filters.userId ||
			filters.agent ||
			filters.surface ||
			filters.minRisk ||
			filters.label ||
			filters.open ||
			filters.query ||
			filters.range !== '24h'
		)
	);

	const surfaceOptions = [
		{ id: '', label: m.supervision_sessions_all_surfaces() },
		...SURFACES.map((s) => ({ id: s, label: surfaceLabel(s) }))
	];
	const labelOptions = [
		{ id: '', label: m.supervision_sessions_all_labels() },
		...CONTENT_LABELS.map((l) => ({ id: l, label: l }))
	];
	const rangeOptions = RANGES.map((r) => ({ id: r, label: rangeLabel(r) }));
	const selectClass =
		'bg-base-100 dark:bg-base-200 dark:border-base-400 border border-transparent shadow-sm';

	let lastPageIndex = $derived(Math.max(0, Math.ceil(total / PAGE_SIZE) - 1));
</script>

<svelte:head>
	<title>Obot | {m.supervision_sessions_title()}</title>
</svelte:head>

<Layout title={m.supervision_sessions_title()}>
	<div class="flex flex-col gap-4 pb-8">
		<p class="text-muted-content text-sm">{m.supervision_sessions_description()}</p>

		<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 flex flex-col gap-2 py-1">
			<Search
				value={filters.query}
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
				onChange={(v) => setParams({ query: v })}
				placeholder={m.supervision_sessions_filter_query_placeholder()}
			/>
			<div class="flex flex-wrap items-end gap-2">
				<div class="flex flex-col gap-1">
					<label for="sessions-user" class="text-xs font-light"
						>{m.supervision_sessions_filter_user()}</label
					>
					<input
						id="sessions-user"
						class="text-input-filled w-40 text-sm"
						placeholder={m.supervision_sessions_filter_user_placeholder()}
						value={filters.userId}
						onchange={(e) => setParams({ user_id: (e.target as HTMLInputElement).value })}
					/>
				</div>
				<div class="flex flex-col gap-1">
					<label for="sessions-agent" class="text-xs font-light"
						>{m.supervision_sessions_filter_agent()}</label
					>
					<input
						id="sessions-agent"
						class="text-input-filled w-40 text-sm"
						placeholder={m.supervision_sessions_filter_agent_placeholder()}
						value={filters.agent}
						onchange={(e) => setParams({ agent: (e.target as HTMLInputElement).value })}
					/>
				</div>
				<div class="flex flex-col gap-1">
					<span id="sessions-surface-label" class="text-xs font-light"
						>{m.supervision_sessions_filter_surface()}</span
					>
					<Select
						id="sessions-surface"
						class={selectClass}
						classes={{ root: 'w-44' }}
						ariaLabelledby="sessions-surface-label"
						options={surfaceOptions}
						selected={filters.surface}
						onSelect={(o) => setParams({ surface: String(o.id) })}
					/>
				</div>
				<div class="flex flex-col gap-1">
					<span id="sessions-label-label" class="text-xs font-light"
						>{m.supervision_sessions_filter_label()}</span
					>
					<Select
						id="sessions-label"
						class={selectClass}
						classes={{ root: 'w-44' }}
						ariaLabelledby="sessions-label-label"
						options={labelOptions}
						selected={filters.label}
						onSelect={(o) => setParams({ label: String(o.id) })}
					/>
				</div>
				<div class="flex flex-col gap-1">
					<label for="sessions-min-risk" class="text-xs font-light"
						>{m.supervision_sessions_filter_min_risk()}</label
					>
					<input
						id="sessions-min-risk"
						type="number"
						min="0"
						max="100"
						class="text-input-filled w-24 text-sm"
						value={filters.minRisk ?? ''}
						onchange={(e) => setParams({ min_risk: (e.target as HTMLInputElement).value })}
					/>
				</div>
				<div class="flex flex-col gap-1">
					<span id="sessions-range-label" class="text-xs font-light"
						>{m.supervision_sessions_filter_range()}</span
					>
					<Select
						id="sessions-range"
						class={selectClass}
						classes={{ root: 'w-40' }}
						ariaLabelledby="sessions-range-label"
						options={rangeOptions}
						selected={filters.range}
						onSelect={(o) => setParams({ range: String(o.id) })}
					/>
				</div>
				<div class="pb-2">
					<Toggle
						label={m.supervision_sessions_filter_open()}
						labelInline
						checked={filters.open}
						onChange={(v) => setParams({ open: v ? 'true' : undefined })}
					/>
				</div>
				{#if hasFilters}
					<button
						type="button"
						class="btn btn-ghost btn-sm text-xs"
						onclick={() => clearUrlParams()}
					>
						{m.supervision_sessions_clear_filters()}
					</button>
				{/if}
			</div>
		</div>

		{#if error}
			<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
		{/if}

		{#if loading && sessions.length === 0}
			<Skeleton type="table" count={6} />
		{:else if rows.length === 0}
			<div class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center">
				<Activity class="text-muted-content size-24 opacity-25" />
				<p class="text-muted-content text-sm">{m.supervision_sessions_empty()}</p>
			</div>
		{:else}
			<Table
				data={rows}
				fields={[
					'risk',
					'name',
					'user',
					'agent',
					'surfaces',
					'labels',
					'events',
					'startedAt',
					'lastActivityAt'
				]}
				headers={[
					{ property: 'risk', title: m.supervision_risk() },
					{ property: 'name', title: m.supervision_col_session() },
					{ property: 'user', title: m.core_col_user() },
					{ property: 'agent', title: m.supervision_col_agent() },
					{ property: 'surfaces', title: m.supervision_col_surfaces() },
					{ property: 'labels', title: m.supervision_labels() },
					{ property: 'events', title: m.supervision_col_events() },
					{ property: 'startedAt', title: m.supervision_col_started() },
					{ property: 'lastActivityAt', title: m.supervision_col_last_activity() }
				]}
				manualSorting
				sortable={['risk', 'lastActivityAt']}
				initSort={{ property: filters.sort, order: filters.sortDirection }}
				onSort={setSortUrlParams}
				noAutoHideFields={['risk', 'name']}
				onClickRow={(row, isCtrl) => openUrl(`/sessions/${row.id}`, isCtrl)}
			>
				{#snippet onRenderColumn(property, row)}
					{#if property === 'risk'}
						<RiskPill risk={row.risk} />
					{:else if property === 'name'}
						<span class="flex flex-col">
							<a href={resolve(`/sessions/${row.id}`)} class="font-medium hover:underline">
								{row.name}
							</a>
							<span class="text-muted-content flex items-center gap-1 text-xs">
								{#if row.killed}
									<span class="badge badge-xs badge-error">{m.supervision_decision_killed()}</span>
								{/if}
								{row.open ? m.supervision_sessions_open() : m.supervision_sessions_ended()}
							</span>
						</span>
					{:else if property === 'surfaces'}
						<span class="flex flex-wrap gap-1">
							{#each row.surfaces as s (s)}
								<SurfaceBadge surface={s} short />
							{/each}
						</span>
					{:else if property === 'labels'}
						<LabelBadges labels={row.labels} max={3} />
					{:else if property === 'startedAt' || property === 'lastActivityAt'}
						{@const t = formatTimeAgo(row[property])}
						<span class="text-xs" title={t.fullDate}>{t.relativeTime}</span>
					{:else}
						{row[property as keyof typeof row]}
					{/if}
				{/snippet}
			</Table>
			{#if total > PAGE_SIZE}
				<Pagination
					pageIndex={filters.pageIndex}
					{lastPageIndex}
					{total}
					{loading}
					label={m.supervision_sessions_title()}
					itemCountLabel={(n) => m.supervision_sessions_count({ count: n })}
					onPageChange={(idx) => setParams({ page: idx ? String(idx) : undefined })}
				/>
			{/if}
		{/if}
	</div>
</Layout>
