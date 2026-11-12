<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import Select from '$lib/components/Select.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		createRoutingRule,
		deleteRoutingRule,
		listProviders,
		parseFallback,
		updateRoutingRule,
		type Provider,
		type RoutingRule
	} from '$lib/services/llm-gateway/gateway';
	import { providerLabel } from '$lib/services/llm-gateway/providers';
	import { success } from '$lib/stores/success';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		rule?: RoutingRule;
		readonly?: boolean;
		onCreate?: (rule?: RoutingRule) => void;
		onUpdate?: (rule?: RoutingRule) => void;
		// Cancel only. Save stays on the form and reports success itself.
		onCancel?: () => void;
	}

	let { rule, readonly = false, onCreate, onUpdate, onCancel }: Props = $props();

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

	let providers = $state<Provider[]>([]);
	let loading = $state(true);
	let name = $state('');
	let description = $state('');
	let enabled = $state(true);
	let cel = $state('');
	let priority = $state(0);
	let targets = $state<TargetRow[]>([]);
	let fallbacks = $state<FallbackRow[]>([]);
	let saving = $state(false);
	let deleting = $state(false);
	let error = $state('');

	let providerOptions = $derived(
		providers.map((p) => ({ id: p.name, label: providerLabel(p.name) }))
	);
	let targetProviderOptions = $derived([
		{ id: SAME_PROVIDER, label: m.models_routing_same_provider() },
		...providerOptions
	]);

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

	function apply(source: RoutingRule | undefined, defaultProvider: string) {
		name = source?.name ?? '';
		description = source?.description ?? '';
		enabled = source?.enabled ?? true;
		cel = source?.cel_expression ?? 'model == "gpt-4o"';
		priority = source?.priority ?? 0;
		targets = (source?.targets ?? [{ provider: defaultProvider, model: '', weight: 1 }]).map(
			(t) => ({
				provider: t.provider ?? '',
				model: t.model ?? '',
				weight: t.weight,
				ttft: t.ttft_timeout_ms ? t.ttft_timeout_ms / 1000 : null,
				keyID: t.key_id
			})
		);
		fallbacks = (source?.fallbacks ?? []).map(parseFallback);
	}

	function errorMessage(e: unknown) {
		const msg = e instanceof Error ? e.message : String(e);
		return /"message":"([^"]+)"/.exec(msg)?.[1] ?? msg;
	}

	onMount(async () => {
		try {
			providers = await listProviders();
		} catch (e) {
			error = errorMessage(e);
		} finally {
			apply(rule, providers[0]?.name ?? '');
			loading = false;
		}
	});

	async function remove() {
		const id = rule?.id;
		if (!id) return;
		saving = true;
		error = '';
		try {
			await deleteRoutingRule(id, { dontLogErrors: true });
			deleting = false;
			close();
		} catch (e) {
			error = errorMessage(e);
			deleting = false;
		} finally {
			saving = false;
		}
	}

	function close() {
		if (onCancel) {
			onCancel();
			return;
		}
		if (rule?.id) onUpdate?.();
		else onCreate?.();
	}

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
			const id = rule?.id;
			const done = id ? onUpdate : onCreate;
			const saved = id
				? await updateRoutingRule(id, body, { dontLogErrors: true })
				: await createRoutingRule(body, { dontLogErrors: true });
			success.add(m.models_routing_saved());
			done?.(saved);
		} catch (e) {
			error = errorMessage(e);
		} finally {
			saving = false;
		}
	}
</script>

{#if loading}
	<Skeleton type="list" count={8} />
{:else}
	<form
		aria-label={rule?.id ? m.models_routing_edit_title() : m.models_routing_add_title()}
		onsubmit={(e) => {
			e.preventDefault();
			save();
		}}
	>
		<div class="flex flex-col gap-6">
			{#if rule?.id}
				<header class="flex w-full items-center justify-between gap-4">
					<div class="flex items-center gap-2">
						<h1 class="flex items-center gap-4 text-2xl font-semibold">
							{name}
						</h1>
					</div>
					{#if !readonly}
						<IconButton
							variant="danger2"
							disabled={saving}
							tooltip={{ text: m.models_routing_delete_rule() }}
							onclick={() => (deleting = true)}
						>
							<Trash2 class="size-4" />
						</IconButton>
					{/if}
				</header>
			{/if}
			<div class="paper">
				<div class="grid grid-cols-1 gap-4 md:grid-cols-3">
					<div class="flex flex-col gap-1 md:col-span-2">
						<label for="rule-name" class="text-sm font-light">{m.core_name()}</label>
						<input id="rule-name" class="text-input-filled" bind:value={name} disabled={readonly} />
					</div>
					<div class="flex flex-col gap-1">
						<label for="rule-priority" class="text-sm font-light"
							>{m.models_routing_priority()}</label
						>
						<input
							id="rule-priority"
							type="number"
							class="text-input-filled"
							bind:value={priority}
							aria-describedby="rule-priority-help"
							disabled={readonly}
						/>
						<p id="rule-priority-help" class="text-muted-content text-xs">
							{m.models_routing_priority_help()}
						</p>
					</div>
					<div class="flex flex-col gap-1 md:col-span-3">
						<label for="rule-description" class="text-sm font-light">{m.core_description()}</label>
						<input
							id="rule-description"
							class="text-input-filled"
							bind:value={description}
							disabled={readonly}
						/>
					</div>
				</div>
			</div>
			<div class="paper">
				<div class="flex flex-col gap-1 md:col-span-3">
					<label for="rule-cel" class="text-sm font-light">{m.models_routing_when_cel()}</label>
					<input
						id="rule-cel"
						class="text-input-filled font-mono text-sm"
						bind:value={cel}
						disabled={readonly}
					/>
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

			<div class="paper">
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
								disabled={readonly}
								onSelect={(option) => (t.provider = option.id === SAME_PROVIDER ? '' : option.id)}
							/>
							<input
								class="text-input-filled min-w-48 flex-1"
								placeholder={m.models_routing_target_model_placeholder()}
								bind:value={t.model}
								aria-label={m.models_routing_target_model()}
								disabled={readonly}
							/>
							<input
								type="number"
								min="0"
								max="1"
								step="0.05"
								class="text-input-filled w-24"
								bind:value={t.weight}
								aria-label={m.models_routing_target_weight()}
								disabled={readonly}
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
								disabled={readonly}
							/>
							{#if !readonly}
								<IconButton
									variant="danger"
									tooltip={{ text: m.core_remove() }}
									onclick={() => targets.splice(i, 1)}
								>
									<Trash2 class="size-4" />
								</IconButton>
							{/if}
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
					{#if !readonly}
						<div class="flex justify-end">
							<button
								type="button"
								class="btn btn-secondary btn-sm flex w-fit items-center gap-1"
								onclick={() =>
									targets.push({
										provider: providers[0]?.name ?? '',
										model: '',
										weight: 0,
										ttft: null
									})}
							>
								<Plus class="size-3" />
								{m.models_routing_add_target()}
							</button>
						</div>
					{/if}
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
								disabled={readonly}
								onSelect={(option) => (f.provider = option.id)}
							/>
							<input
								class="text-input-filled min-w-48 flex-1"
								placeholder={m.models_routing_model()}
								bind:value={f.model}
								aria-label={m.models_routing_fallback_model()}
								disabled={readonly}
							/>
							{#if !readonly}
								<IconButton
									variant="danger"
									tooltip={{ text: m.core_remove() }}
									onclick={() => fallbacks.splice(i, 1)}
								>
									<Trash2 class="size-4" />
								</IconButton>
							{/if}
						</div>
					{/each}
					{#if !readonly}
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
					{/if}
				</div>
			</div>

			{#if error}
				<div class="notification-error rounded-md p-2 text-sm">{error}</div>
			{/if}
		</div>

		{#if !readonly}
			<div class="bg-base-200 dark:bg-base-100 sticky bottom-0 left-0 z-50 flex w-full py-4">
				<div class="flex w-full justify-between gap-2">
					<div>
						{#if rule?.id}
							<button
								type="button"
								class={twMerge('text-sm btn btn-soft', enabled ? 'btn-error' : 'btn-primary')}
								disabled={saving}
								use:tooltip={m.models_routing_enabled_hint()}
								onclick={() => (enabled = !enabled)}
							>
								{enabled ? m.models_routing_disable() : m.models_routing_enable()}
							</button>
						{/if}
					</div>
					<div class="flex gap-2">
						<button type="button" class="btn btn-secondary text-sm" onclick={close}>
							{m.common_cancel()}
						</button>
						<button type="submit" class="btn btn-primary text-sm" disabled={!valid || saving}>
							{#if saving}<Loading class="size-4" />{:else}{m.core_save()}{/if}
						</button>
					</div>
				</div>
			</div>
		{/if}
	</form>

	<Confirm
		msg={m.models_routing_delete_confirm({ name: name.trim() || rule?.name || '' })}
		show={deleting}
		loading={saving}
		onsuccess={remove}
		oncancel={() => (deleting = false)}
	/>
{/if}
