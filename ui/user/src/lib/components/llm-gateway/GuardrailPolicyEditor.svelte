<script lang="ts">
	import { resolve } from '$app/paths';
	import Select from '$lib/components/Select.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import NewGuardrailDialog from '$lib/components/guardrails/NewGuardrailDialog.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { guardrailTypeLabel } from '$lib/services/llm-gateway/guardrail-text';
	import {
		ACTION_LABELS,
		APPLY_TO_LABELS,
		listGuardrails,
		listGuardrailProviders,
		validateGuardrailCondition,
		type GuardrailAction,
		type GuardrailApplyTo,
		type GuardrailInputScope,
		type Guardrail,
		type GuardrailProvider,
		type GuardrailPolicy
	} from '$lib/services/llm-gateway/guardrails';
	import { Plus, TriangleAlert } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';

	// The fields of one guardrail policy of a profile, shown as an applied guardrail. It edits a
	// copy and hands the result to onSave; the profile form keeps it in its draft until the
	// profile is saved.
	interface Props {
		// The policy to edit, or undefined for a new one.
		policy?: GuardrailPolicy;
		readonly?: boolean;
		onSave: (policy: GuardrailPolicy, guardrails: Guardrail[]) => void;
		onCancel: () => void;
	}

	let { policy: initial, readonly = false, onSave, onCancel }: Props = $props();

	// Policies of a profile guard model traffic. MCP guardrails are configured as MCP server filters.
	const TARGET = 'llm';

	const conditionVariables = [
		{ name: 'model', description: m.models_guardrail_policies_var_model() },
		{ name: 'provider', description: m.models_guardrail_policies_var_provider() },
		{ name: 'params', description: m.models_guardrail_policies_var_params() },
		{ name: 'headers', description: m.models_guardrail_policies_var_headers() },
		{ name: 'user', description: m.models_guardrail_policies_var_user() }
	];
	const conditionExample = 'model.startsWith("openai/") && !("<group ID>" in user.groups)';

	const applyToOptions = Object.entries(APPLY_TO_LABELS).map(([id, label]) => ({ id, label }));
	const actionOptions = Object.entries(ACTION_LABELS).map(([id, label]) => ({ id, label }));
	const inputScopeOptions = [
		{ id: 'latest', label: m.models_guardrail_policies_input_scope_latest() },
		{ id: 'all', label: m.models_guardrail_policies_input_scope_all() }
	];
	const selectClass =
		'bg-base-200 dark:bg-base-200 dark:border-base-400 border border-transparent shadow-inner';

	const start = untrack(() => initial);
	// A policy that is saved already; a new one starts enabled.
	const saved = !!start?.id;

	let name = $state(start?.name ?? '');
	let description = $state(start?.description ?? '');
	let enabled = $state(start?.enabled ?? true);
	let applyTo = $state<GuardrailApplyTo>(start?.applyTo ?? 'both');
	let action = $state<GuardrailAction>(start?.action ?? 'block');
	let condition = $state(start?.condition ?? '');
	let samplingRate = $state(start?.samplingRate || 100);
	let inputScope = $state<GuardrailInputScope>(start?.inputScope || 'latest');
	let failClosed = $state(start?.failClosed ?? false);
	let blockMessage = $state(start?.blockMessage ?? '');
	let guardrailIDs = $state<string[]>(start?.guardrailIDs ?? []);

	let guardrails = $state<Guardrail[]>([]);
	let providers = $state<GuardrailProvider[]>([]);
	let loadError = $state('');
	let newGuardrailDialog = $state<ReturnType<typeof NewGuardrailDialog>>();
	let conditionError = $state('');
	let validating = $state(false);

	onMount(async () => {
		try {
			[guardrails, providers] = await Promise.all([listGuardrails(), listGuardrailProviders()]);
		} catch (e) {
			loadError = parseErrorContent(e).message;
		}
	});

	// Validate the condition as it's typed, ignoring answers superseded by later edits.
	let validationSeq = 0;
	$effect(() => {
		const expr = condition.trim();
		const seq = ++validationSeq;
		if (!expr) {
			conditionError = '';
			validating = false;
			return;
		}
		validating = true;
		const timer = setTimeout(async () => {
			try {
				const result = await validateGuardrailCondition(TARGET, expr, { dontLogErrors: true });
				if (seq === validationSeq) conditionError = result.valid ? '' : (result.error ?? '');
			} catch (e) {
				if (seq === validationSeq) conditionError = parseErrorContent(e).message;
			} finally {
				if (seq === validationSeq) validating = false;
			}
		}, 400);
		return () => clearTimeout(timer);
	});

	let providersByType = $derived(Object.fromEntries(providers.map((p) => [p.type, p])));
	let nonRedacting = $derived(
		action === 'redact'
			? guardrails.filter(
					(p) => p.id && guardrailIDs.includes(p.id) && providersByType[p.type]?.redacts === false
				)
			: []
	);

	let valid = $derived(
		name.trim().length > 0 &&
			guardrailIDs.length > 0 &&
			samplingRate >= 1 &&
			samplingRate <= 100 &&
			!conditionError &&
			!validating
	);

	function save() {
		onSave(
			{
				...(start ?? {}),
				name: name.trim(),
				description,
				enabled,
				target: TARGET,
				applyTo,
				action,
				condition: condition.trim(),
				samplingRate: Number(samplingRate),
				inputScope,
				failClosed,
				blockMessage: blockMessage.trim(),
				guardrailIDs,
				profileID: start?.profileID ?? ''
			},
			guardrails
		);
	}
</script>

<div class="flex flex-col gap-4">
	<p class="text-muted-content text-sm">{m.models_guardrail_policies_details_description()}</p>
	{#if saved}
		<div>
			<Toggle
				label={m.core_status_enabled()}
				labelInline
				checked={enabled}
				disabled={readonly}
				onChange={(v) => (enabled = v)}
			/>
			<p class="text-muted-content mt-1 text-xs">{m.models_guardrail_policies_enabled_hint()}</p>
		</div>
	{/if}
	<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
		<div class="flex flex-col gap-1 md:col-span-2">
			<label for="policy-name" class="text-sm font-light">{m.core_name()}</label>
			<input id="policy-name" class="text-input-filled" bind:value={name} disabled={readonly} />
		</div>
		<div class="flex flex-col gap-1 md:col-span-2">
			<label for="policy-description" class="text-sm font-light">{m.core_description()}</label>
			<input
				id="policy-description"
				class="text-input-filled"
				bind:value={description}
				disabled={readonly}
			/>
		</div>
		<div class="flex flex-col gap-1">
			<span id="policy-apply-to-label" class="text-sm font-light"
				>{m.models_guardrail_policies_apply_to()}</span
			>
			<Select
				id="policy-apply-to"
				class={selectClass}
				ariaLabelledby="policy-apply-to-label"
				options={applyToOptions}
				selected={applyTo}
				onSelect={(option) => (applyTo = option.id as GuardrailApplyTo)}
				disabled={readonly}
			/>
			<span class="text-muted-content text-xs">
				{m.models_guardrail_policies_apply_to_description()}
			</span>
		</div>
		<div class="flex flex-col gap-1">
			<span id="policy-action-label" class="text-sm font-light"
				>{m.models_guardrail_policies_action()}</span
			>
			<Select
				id="policy-action"
				class={selectClass}
				ariaLabelledby="policy-action-label"
				options={actionOptions}
				selected={action}
				onSelect={(option) => (action = option.id as GuardrailAction)}
				disabled={readonly}
			/>
			<span class="text-muted-content text-xs">
				{action === 'block'
					? m.models_guardrail_policies_action_block_description()
					: action === 'redact'
						? m.models_guardrail_policies_action_redact_description()
						: m.models_guardrail_policies_action_log_description()}
			</span>
		</div>
		<div class="flex flex-col gap-1">
			<label for="policy-sampling" class="text-sm font-light"
				>{m.models_guardrail_policies_sampling_rate()}</label
			>
			<input
				id="policy-sampling"
				type="number"
				min="1"
				max="100"
				class="text-input-filled"
				bind:value={samplingRate}
				disabled={readonly}
			/>
			<span class="text-muted-content text-xs">
				{m.models_guardrail_policies_sampling_rate_description()}
			</span>
		</div>
		{#if applyTo !== 'output'}
			<div class="flex flex-col gap-1">
				<span id="policy-input-scope-label" class="text-sm font-light"
					>{m.models_guardrail_policies_input_scope()}</span
				>
				<Select
					id="policy-input-scope"
					class={selectClass}
					ariaLabelledby="policy-input-scope-label"
					options={inputScopeOptions}
					selected={inputScope}
					onSelect={(option) => (inputScope = option.id as GuardrailInputScope)}
					disabled={readonly}
				/>
				<span class="text-muted-content text-xs">
					{m.models_guardrail_policies_input_scope_description()}
				</span>
			</div>
		{/if}
	</div>
	<Toggle
		label={m.models_guardrail_policies_fail_closed()}
		labelInline
		checked={failClosed}
		disabled={readonly}
		onChange={(v) => (failClosed = v)}
	/>
	{#if action !== 'log'}
		<div class="flex flex-col gap-1">
			<label for="policy-block-message" class="text-sm font-light"
				>{m.models_guardrail_policies_block_message()}</label
			>
			<input
				id="policy-block-message"
				class="text-input-filled"
				placeholder={m.models_guardrail_policies_block_message_placeholder()}
				bind:value={blockMessage}
				disabled={readonly}
			/>
		</div>
	{/if}

	<section class="flex flex-col gap-3">
		<div class="divider my-1 text-sm font-semibold">
			{m.models_guardrail_policies_guardrails()}
		</div>
		<div class="flex flex-wrap items-start justify-between gap-2">
			<p class="text-muted-content max-w-xl text-xs">
				{m.models_guardrail_policies_guardrails_description()}
			</p>
			<div class="flex items-center gap-3">
				<a href={resolve('/models?view=guardrails')} class="text-link text-sm">
					{m.guardrails_manage()}
				</a>
				{#if !readonly}
					<button
						type="button"
						class="btn btn-secondary btn-sm flex items-center gap-1 text-xs"
						onclick={() => newGuardrailDialog?.open()}
					>
						<Plus class="size-4" />
						{m.guardrails_new()}
					</button>
				{/if}
			</div>
		</div>
		{#if loadError}
			<p class="text-error text-xs">{loadError}</p>
		{/if}
		<div class="grid grid-cols-1 gap-1 md:grid-cols-2">
			{#each guardrails as p (p.id)}
				<label class="flex items-center gap-2 text-sm">
					<input
						type="checkbox"
						class="checkbox checkbox-sm"
						checked={!!p.id && guardrailIDs.includes(p.id)}
						disabled={readonly}
						onchange={(e) => {
							const checked = (e.target as HTMLInputElement).checked;
							guardrailIDs = checked
								? [...guardrailIDs, p.id!]
								: guardrailIDs.filter((x) => x !== p.id);
						}}
					/>
					<span>{p.name}</span>
					<span class="text-muted-content text-xs">
						{providersByType[p.type] ? guardrailTypeLabel(providersByType[p.type]) : p.type}
					</span>
				</label>
			{:else}
				<span class="text-muted-content text-sm">{m.models_guardrail_policies_no_guardrails()}</span
				>
			{/each}
		</div>
		{#if nonRedacting.length}
			<div class="notification-alert flex items-start gap-2 rounded-md p-3 text-sm">
				<TriangleAlert class="size-4 shrink-0" />
				<span>
					{nonRedacting.length === 1
						? m.models_guardrail_policies_non_redacting_one({ names: nonRedacting[0].name })
						: m.models_guardrail_policies_non_redacting_other({
								names: nonRedacting.map((p) => p.name).join(', ')
							})}
				</span>
			</div>
		{/if}
	</section>

	<section class="flex flex-col gap-3">
		<div class="divider my-1 text-sm font-semibold">
			{m.models_guardrail_policies_condition()}
		</div>
		<p class="text-muted-content text-xs">
			{m.models_guardrail_policies_condition_description_prefix()}
			<a href="https://cel.dev" target="_blank" rel="noopener noreferrer" class="text-link">CEL</a>
			{m.models_guardrail_policies_condition_description_suffix()}
		</p>
		<textarea
			id="policy-condition"
			aria-label={m.models_guardrail_policies_condition()}
			class="text-input-filled min-h-20 font-mono text-sm"
			placeholder={conditionExample}
			bind:value={condition}
			disabled={readonly}></textarea>
		{#if conditionError}
			<span class="text-error text-xs" role="alert">{conditionError}</span>
		{/if}
		<div class="text-muted-content grid grid-cols-1 gap-x-4 gap-y-1 text-xs md:grid-cols-2">
			{#each conditionVariables as v (v.name)}
				<span><code class="text-base-content">{v.name}</code> — {v.description}</span>
			{/each}
		</div>
	</section>

	<div class="flex justify-end gap-2 pt-2">
		{#if readonly}
			<button type="button" class="btn btn-secondary text-sm" onclick={onCancel}
				>{m.core_close()}</button
			>
		{:else}
			<button type="button" class="btn btn-secondary text-sm" onclick={onCancel}
				>{m.common_cancel()}</button
			>
			<button type="button" class="btn btn-primary text-sm" disabled={!valid} onclick={save}>
				{m.common_apply()}
			</button>
		{/if}
	</div>
</div>

<NewGuardrailDialog
	bind:this={newGuardrailDialog}
	onCreated={(g) => {
		guardrails = [...guardrails, g];
		if (g.id) guardrailIDs = [...guardrailIDs, g.id];
	}}
/>
