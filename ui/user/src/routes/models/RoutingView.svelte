<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import GatewayStatusBanner from '$lib/components/llm-gateway/GatewayStatusBanner.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import {
		deleteRoutingRule,
		getStatus,
		listRoutingRules,
		parseFallback,
		type GatewayStatus,
		type RoutingRule
	} from '$lib/services/llm-gateway/gateway';
	import { providerLabel } from '$lib/services/llm-gateway/providers';
	import { profile } from '$lib/stores';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		goto,
		replaceState,
		setFilterUrlParams,
		setSortUrlParams
	} from '$lib/url';
	import { openUrl } from '$lib/utils';
	import RoutingForm from './routing/RoutingForm.svelte';
	import { GitFork, Trash2 } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		// Reports whether the gateway is running, so the page can offer actions that need it.
		onReadyChange?: (ready: boolean) => void;
	}

	let { onReadyChange }: Props = $props();

	let status = $state<GatewayStatus>();
	let rules = $state<RoutingRule[]>([]);
	let loading = $state(true);
	let toDelete = $state<RoutingRule>();

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let showCreate = $derived(page.url.searchParams.has('new') && !readonly);

	async function load() {
		loading = true;
		try {
			status = await getStatus();
			onReadyChange?.(!!status.ready);
			if (!status.ready) return;
			rules = await listRoutingRules();
			rules.sort((a, b) => (a.priority ?? 0) - (b.priority ?? 0));
		} finally {
			loading = false;
		}
	}

	onMount(load);

	function listUrl() {
		return '/models?view=routing';
	}

	function targetLabel(r: RoutingRule) {
		return r.targets
			.map((t) => {
				const target = {
					provider: t.provider ? providerLabel(t.provider) : m.models_routing_same_provider_lower(),
					model: t.model || m.models_routing_same_model_lower(),
					percent: Math.round(t.weight * 100)
				};
				return t.ttft_timeout_ms
					? m.models_routing_target_summary_ttft({ ...target, seconds: t.ttft_timeout_ms / 1000 })
					: m.models_routing_target_summary(target);
			})
			.join(', ');
	}

	let rows = $derived(
		rules.map((r) => ({
			...r,
			id: r.id ?? r.name,
			targetsLabel: targetLabel(r),
			fallbacksLabel:
				(r.fallbacks ?? [])
					.map((f) => {
						const p = parseFallback(f);
						return `${p.provider}/${p.model}`;
					})
					.join(' → ') || '—',
			enabledLabel: r.enabled === false ? 'Disabled' : 'Enabled'
		}))
	);

	// Status values stay in English in the rows, because filters are saved in URLs;
	// statusDisplay translates them only for display.
	function statusDisplay(property: string, value: unknown): string {
		if (property !== 'enabledLabel') return String(value ?? '');
		return value === 'Disabled' ? m.core_status_disabled() : m.core_status_enabled();
	}

	let query = $state(page.url.searchParams.get('query') || '');
	let filteredRows = $derived(
		rows.filter((r) =>
			[r.name, r.cel_expression, r.targetsLabel].some((v) =>
				v?.toLowerCase().includes(query.toLowerCase())
			)
		)
	);
	let urlFilters = $derived(getTableUrlParamsFilters());
	let initSort = $derived(getTableUrlParamsSort({ property: 'priority', order: 'asc' }));

	function updateQuery(value: string) {
		query = value;
		const url = new URL(page.url.toString());
		if (value) {
			url.searchParams.set('query', value);
		} else {
			url.searchParams.delete('query');
		}
		replaceState(url, { query: value });
	}

	function clearFilters() {
		clearUrlParams(Array.from(page.url.searchParams.keys()).filter((key) => key !== 'view'));
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

{#if showCreate && status?.ready}
	<div in:fly|global={{ x: 100, duration }} out:fly|global={{ x: -100, duration }}>
		<RoutingForm onCreate={() => goto(listUrl(), { replaceState: true })} />
	</div>
{:else}
	<div class="flex flex-col gap-4">
		<GatewayStatusBanner {status} showNoProviders={false} />

		{#if loading}
			<Skeleton
				type="table"
				count={5}
				classes={{ header: 'h-14 rounded-none', body: 'rounded-none' }}
			/>
		{:else if status?.ready && rules.length === 0}
			<div class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center">
				<GitFork class="text-muted-content size-24 opacity-25" />
				<h4 class="text-muted-content text-lg font-semibold">{m.models_routing_empty_title()}</h4>
				<p class="text-muted-content text-sm font-light">
					{m.models_routing_empty_description()}
				</p>
			</div>
		{:else if status?.ready}
			<div class="flex flex-col gap-2">
				<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
					<Search
						value={query}
						class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
						onChange={updateQuery}
						placeholder={m.models_routing_search_placeholder()}
					/>
				</div>
				<Table
					data={filteredRows}
					fields={[
						'priority',
						'name',
						'cel_expression',
						'targetsLabel',
						'fallbacksLabel',
						'enabledLabel'
					]}
					headers={[
						{ property: 'priority', title: m.models_routing_priority() },
						{ property: 'name', title: m.core_name() },
						{ property: 'cel_expression', title: m.models_routing_when() },
						{ property: 'targetsLabel', title: m.models_routing_route_to() },
						{ property: 'fallbacksLabel', title: m.models_routing_fallbacks() },
						{ property: 'enabledLabel', title: m.core_status() }
					]}
					sortable={['priority', 'name', 'enabledLabel']}
					filterable={['enabledLabel']}
					filters={urlFilters}
					displayValue={statusDisplay}
					onFilter={(property, values) => setFilterUrlParams({ [property]: values })}
					onClearAllFilters={clearFilters}
					onSort={setSortUrlParams}
					{initSort}
					onClickRow={(row, isCtrlClick) =>
						openUrl(`/models/routing/${encodeURIComponent(row.id)}`, isCtrlClick)}
				>
					{#snippet onRenderColumn(property, row)}
						{#if property === 'name'}
							<a
								href={`/models/routing/${encodeURIComponent(row.id)}`}
								class="text-base-content"
								onclick={(e) => e.stopPropagation()}
							>
								{row.name}
							</a>
						{:else if property === 'cel_expression'}
							<code class="text-xs">{row.cel_expression}</code>
						{:else}
							{statusDisplay(property, row[property as keyof typeof row])}
						{/if}
					{/snippet}
					{#snippet actions(row)}
						{#if !readonly}
							<IconButton
								variant="danger"
								tooltip={{ text: m.models_routing_delete_rule() }}
								onclick={(e) => {
									e.stopPropagation();
									toDelete = row;
								}}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					{/snippet}
				</Table>
			</div>
		{/if}
	</div>
{/if}

<Confirm
	show={!!toDelete}
	msg={m.models_routing_delete_confirm({ name: toDelete?.name ?? '' })}
	onsuccess={async () => {
		if (toDelete?.id) await deleteRoutingRule(toDelete.id);
		toDelete = undefined;
		await load();
	}}
	oncancel={() => (toDelete = undefined)}
/>
