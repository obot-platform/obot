<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import GatewayStatusBanner from '$lib/components/llm-gateway/GatewayStatusBanner.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		createRoutingRule,
		deleteRoutingRule,
		getStatus,
		listProviders,
		listRoutingRules,
		parseFallback,
		updateRoutingRule,
		type GatewayStatus,
		type Provider,
		type RoutingRule
	} from '$lib/services/llm-gateway/gateway';
	import { providerLabel } from '$lib/services/llm-gateway/providers';
	import { profile } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import {
		clearUrlParams,
		getTableUrlParamsFilters,
		getTableUrlParamsSort,
		replaceState,
		setFilterUrlParams,
		setSortUrlParams
	} from '$lib/url';
	import { GitFork, Pencil, Plus, Trash2 } from '@lucide/svelte';
	import { onMount } from 'svelte';

	interface Props {
		// Reports whether the gateway is running, so the page can offer actions that need it.
		onReadyChange?: (ready: boolean) => void;
	}

	let { onReadyChange }: Props = $props();

	// Select needs a truthy option id, so "keep the requested provider" gets a sentinel.
	const SAME_PROVIDER = '__same__';

	// The gateway caps a target's time to first token timeout at 300 seconds.
	const MAX_TTFT_SECONDS = 300;

	// ttft is the time to first token timeout in seconds; null when unset.
	// keyID is a target's pin to one provider key; the form doesn't edit it, but keeps it.
	type TargetRow = {
		provider: string;
		model: string;
		weight: number;
		ttft: number | null;
		keyID?: string | null;
	};
	type FallbackRow = { provider: string; model: string };

	// What the gateway's routing engine fills in for each request. The proxy forwards requests
	// to it without their query string, so the engine's params variable is always empty and isn't
	// offered here.
	const celVariables = [
		{ name: 'model', description: m.models_routing_var_model() },
		{ name: 'provider', description: m.models_routing_var_provider() },
		{ name: 'headers', description: m.models_routing_var_headers() },
		{ name: 'request_type', description: m.models_routing_var_request_type() }
	];
	const celExample = 'provider == "openai" && headers["x-tier"] == "premium"';

	let status = $state<GatewayStatus>();
	let rules = $state<RoutingRule[]>([]);
	let providers = $state<Provider[]>([]);
	let loading = $state(true);
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let toDelete = $state<RoutingRule>();

	let editing = $state<RoutingRule>();
	let name = $state('');
	let description = $state('');
	let enabled = $state(true);
	let cel = $state('');
	let priority = $state(0);
	let targets = $state<TargetRow[]>([]);
	let fallbacks = $state<FallbackRow[]>([]);
	let saving = $state(false);
	let error = $state('');

	let readonly = $derived(profile.current.isAdminReadonly?.());

	async function load() {
		loading = true;
		try {
			status = await getStatus();
			onReadyChange?.(!!status.ready);
			if (!status.ready) return;
			[rules, providers] = await Promise.all([listRoutingRules(), listProviders()]);
			rules.sort((a, b) => (a.priority ?? 0) - (b.priority ?? 0));
		} finally {
			loading = false;
		}
	}

	onMount(load);

	export function open(rule?: RoutingRule) {
		editing = rule;
		error = '';
		name = rule?.name ?? '';
		description = rule?.description ?? '';
		enabled = rule?.enabled ?? true;
		cel = rule?.cel_expression ?? 'model == "gpt-4o"';
		priority = rule?.priority ?? 0;
		targets = (rule?.targets ?? [{ provider: providers[0]?.name ?? '', model: '', weight: 1 }]).map(
			(t) => ({
				provider: t.provider ?? '',
				model: t.model ?? '',
				weight: t.weight,
				ttft: t.ttft_timeout_ms ? t.ttft_timeout_ms / 1000 : null,
				keyID: t.key_id
			})
		);
		fallbacks = (rule?.fallbacks ?? []).map(parseFallback);
		dialog?.open();
	}

	let weightSum = $derived(targets.reduce((s, t) => s + Number(t.weight || 0), 0));
	let weightsValid = $derived(Math.abs(weightSum - 1) < 0.0001);
	// An empty input binds null (or '' before the first edit); either means no timeout.
	const hasTTFT = (t: TargetRow) => t.ttft !== null && String(t.ttft) !== '';
	const ttftMs = (t: TargetRow) => Math.round(Number(t.ttft) * 1000);
	let ttftValid = $derived(
		targets.every((t) => !hasTTFT(t) || (ttftMs(t) >= 1 && ttftMs(t) <= MAX_TTFT_SECONDS * 1000))
	);
	let valid = $derived(
		name.trim().length > 0 &&
			cel.trim().length > 0 &&
			targets.length > 0 &&
			weightsValid &&
			ttftValid
	);

	async function save() {
		saving = true;
		error = '';
		try {
			const body: RoutingRule = {
				name: name.trim(),
				description,
				enabled,
				cel_expression: cel.trim(),
				priority: Number(priority),
				scope: 'global',
				targets: targets.map((t) => ({
					provider: t.provider || null,
					model: t.model.trim() || null,
					weight: Number(t.weight),
					...(t.keyID ? { key_id: t.keyID } : {}),
					...(hasTTFT(t) ? { ttft_timeout_ms: ttftMs(t) } : {})
				})),
				fallbacks: fallbacks.filter((f) => f.provider && f.model.trim())
			};
			if (editing?.id) {
				await updateRoutingRule(editing.id, body, { dontLogErrors: true });
			} else {
				await createRoutingRule(body, { dontLogErrors: true });
			}
			dialog?.close();
			success.add(m.models_routing_saved());
			await load();
		} catch (e) {
			const msg = e instanceof Error ? e.message : String(e);
			error = /"message":"([^"]+)"/.exec(msg)?.[1] ?? msg;
		} finally {
			saving = false;
		}
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

	let providerOptions = $derived(
		providers.map((p) => ({ id: p.name, label: providerLabel(p.name) }))
	);
	let targetProviderOptions = $derived([
		{ id: SAME_PROVIDER, label: m.models_routing_same_provider() },
		...providerOptions
	]);

	function updateQuery(value: string) {
		query = value;
		if (value) {
			page.url.searchParams.set('query', value);
		} else {
			page.url.searchParams.delete('query');
		}
		replaceState(page.url, { query: value });
	}

	function clearFilters() {
		clearUrlParams(Array.from(page.url.searchParams.keys()).filter((key) => key !== 'view'));
	}
</script>

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
				onFilter={setFilterUrlParams}
				onClearAllFilters={clearFilters}
				onSort={setSortUrlParams}
				{initSort}
				onClickRow={readonly
					? undefined
					: (row) => open(rules.find((r) => (r.id ?? r.name) === row.id))}
			>
				{#snippet onRenderColumn(property, row)}
					{#if property === 'cel_expression'}
						<code class="text-xs">{row.cel_expression}</code>
					{:else}
						{statusDisplay(property, row[property as keyof typeof row])}
					{/if}
				{/snippet}
				{#snippet actions(row)}
					{#if !readonly}
						<IconButton
							tooltip={{ text: m.models_routing_edit_rule() }}
							onclick={(e) => {
								e.stopPropagation();
								open(rules.find((r) => (r.id ?? r.name) === row.id));
							}}
						>
							<Pencil class="size-4" />
						</IconButton>
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

<ResponsiveDialog
	bind:this={dialog}
	title={editing ? m.models_routing_edit_title() : m.models_routing_add_title()}
	class="max-w-3xl"
>
	{#snippet titleContent()}
		{editing ? m.models_routing_edit_title() : m.models_routing_add_title()}
		<!-- New rules start enabled; the toggle only matters once a rule exists. -->
		{#if editing}
			<span class="ml-2 text-sm font-normal" use:tooltip={m.models_routing_enabled_hint()}>
				<Toggle
					label={m.core_status_enabled()}
					labelInline
					checked={enabled}
					onChange={(v) => (enabled = v)}
				/>
			</span>
		{/if}
	{/snippet}
	<form
		class="flex flex-col gap-4 p-4 md:p-0"
		onsubmit={(e) => {
			e.preventDefault();
			save();
		}}
	>
		<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
			<div class="flex flex-col gap-1 md:col-span-2">
				<label for="rule-name" class="text-sm font-light">{m.core_name()}</label>
				<input id="rule-name" class="text-input-filled" bind:value={name} />
			</div>
			<div class="flex flex-col gap-1">
				<label for="rule-priority" class="text-sm font-light">{m.models_routing_priority()}</label>
				<input id="rule-priority" type="number" class="text-input-filled" bind:value={priority} />
			</div>
			<div class="flex flex-col gap-1 md:col-span-3">
				<label for="rule-description" class="text-sm font-light">{m.core_description()}</label>
				<input id="rule-description" class="text-input-filled" bind:value={description} />
			</div>
			<div class="flex flex-col gap-1 md:col-span-3">
				<label for="rule-cel" class="text-sm font-light">{m.models_routing_when_cel()}</label>
				<input id="rule-cel" class="text-input-filled font-mono text-sm" bind:value={cel} />
				<div class="alert bg-primary/10 border-primary/50 text-xs">
					<div class="flex flex-col gap-0.5">
						{#each celVariables as v (v.name)}
							<div class="grid grid-cols-4 gap-0 md:gap-8 mb-2 md:mb-0">
								<code class="text-base-content col-span-4 md:col-span-1">{v.name}</code>
								<span class="col-span-4 md:col-span-3">{v.description}</span>
							</div>
						{/each}
						<span class="text-muted-content text-xs mt-4">
							{m.models_routing_cel_example()} <code class="text-base-content">{celExample}</code>
						</span>
						<span class="text-muted-content text-xs">
							{m.models_routing_evaluation_order()}
						</span>
					</div>
				</div>
			</div>
		</div>

		<div class="flex flex-col gap-2">
			<span class="text-sm font-medium">{m.models_routing_route_to()}</span>
			<!-- Column headers line up with the row inputs; rows wrap below lg, so they hide there. -->
			<div class="text-muted-content hidden items-end gap-2 text-xs lg:flex" aria-hidden="true">
				<span class="w-48">{m.core_col_provider()}</span>
				<span class="min-w-48 flex-1">{m.models_routing_model()}</span>
				<span class="flex w-24 flex-col">
					{m.models_routing_col_weight()}
					<span class={weightsValid ? '' : 'text-error'}>
						{m.models_routing_weights_total({ sum: weightSum.toFixed(2) })}
					</span>
				</span>
				<span class="flex w-36 items-center gap-1">
					{m.models_routing_col_ttft()}
					<InfoTooltip text={m.models_routing_ttft_help()} />
				</span>
				<span class="w-10 shrink-0"></span>
			</div>
			{#each targets as t, i (i)}
				<div class="flex flex-wrap items-center gap-2">
					<Select
						id={`target-provider-${i}`}
						class="bg-base-200 dark:bg-base-200 dark:border-base-400 border border-transparent shadow-inner"
						classes={{ root: 'w-48' }}
						placeholder={m.models_routing_target_provider()}
						options={targetProviderOptions}
						selected={t.provider || SAME_PROVIDER}
						onSelect={(option) => (t.provider = option.id === SAME_PROVIDER ? '' : option.id)}
					/>
					<input
						class="text-input-filled min-w-48 flex-1"
						placeholder={m.models_routing_target_model_placeholder()}
						bind:value={t.model}
						aria-label={m.models_routing_target_model()}
					/>
					<input
						type="number"
						min="0"
						max="1"
						step="0.05"
						class="text-input-filled w-24"
						bind:value={t.weight}
						aria-label={m.models_routing_target_weight()}
					/>
					<input
						type="number"
						min="0.001"
						max={MAX_TTFT_SECONDS}
						step="any"
						class="text-input-filled w-36"
						placeholder={m.models_routing_ttft_placeholder()}
						title={m.models_routing_ttft_title()}
						bind:value={t.ttft}
						aria-label={m.models_routing_ttft_label()}
					/>
					<IconButton
						variant="danger"
						tooltip={{ text: m.core_remove() }}
						onclick={() => targets.splice(i, 1)}
					>
						<Trash2 class="size-4" />
					</IconButton>
				</div>
			{/each}
			{#if !weightsValid}
				<span class="text-error text-xs">
					{m.models_routing_weights_sum({ sum: weightSum.toFixed(2) })}
				</span>
			{/if}
			{#if !ttftValid}
				<span class="text-error text-xs">
					{m.models_routing_ttft_invalid({ max: MAX_TTFT_SECONDS })}
				</span>
			{/if}
			<div class="flex justify-end">
				<button
					type="button"
					class="btn btn-secondary btn-sm flex w-fit items-center gap-1"
					onclick={() =>
						targets.push({ provider: providers[0]?.name ?? '', model: '', weight: 0, ttft: null })}
				>
					<Plus class="size-3" />
					{m.models_routing_add_target()}
				</button>
			</div>
		</div>

		<div class="flex flex-col gap-2">
			<span class="text-sm font-medium">{m.models_routing_fallbacks()}</span>
			<span class="text-muted-content text-xs">{m.models_routing_fallbacks_help()}</span>
			{#if ttftValid && fallbacks.length === 0 && targets.some(hasTTFT)}
				<span class="text-warning text-xs">
					{m.models_routing_ttft_needs_fallback()}
				</span>
			{/if}
			{#each fallbacks as f, i (i)}
				<div class="flex flex-wrap items-center gap-2">
					<Select
						id={`fallback-provider-${i}`}
						class="bg-base-200 dark:bg-base-200 dark:border-base-400 border border-transparent shadow-inner"
						classes={{ root: 'w-48' }}
						placeholder={m.models_routing_fallback_provider()}
						options={providerOptions}
						selected={f.provider}
						onSelect={(option) => (f.provider = option.id)}
					/>
					<input
						class="text-input-filled min-w-48 flex-1"
						placeholder={m.models_routing_model()}
						bind:value={f.model}
						aria-label={m.models_routing_fallback_model()}
					/>
					<IconButton
						variant="danger"
						tooltip={{ text: m.core_remove() }}
						onclick={() => fallbacks.splice(i, 1)}
					>
						<Trash2 class="size-4" />
					</IconButton>
				</div>
			{/each}
			<div class="flex justify-end">
				<button
					type="button"
					class="btn btn-secondary btn-sm flex w-fit items-center gap-1"
					onclick={() => fallbacks.push({ provider: providers[0]?.name ?? '', model: '' })}
				>
					<Plus class="size-3" />
					{m.models_routing_add_fallback()}
				</button>
			</div>
		</div>

		{#if error}
			<div class="notification-error rounded-md p-2 text-sm">{error}</div>
		{/if}

		<div class="flex justify-end gap-2 mt-6">
			<button type="button" class="btn btn-secondary text-sm" onclick={() => dialog?.close()}>
				{m.common_cancel()}
			</button>
			<button type="submit" class="btn btn-primary text-sm" disabled={!valid || saving}>
				{#if saving}<Loading class="size-4" />{:else}{m.core_save()}{/if}
			</button>
		</div>
	</form>
</ResponsiveDialog>

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
