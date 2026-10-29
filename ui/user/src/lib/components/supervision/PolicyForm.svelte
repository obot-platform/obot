<script lang="ts">
	import Select from '$lib/components/Select.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		approvalTTLFromSeconds,
		approvalTTLSeconds,
		createPolicy,
		describeAction,
		draftPolicy,
		EFFECTS,
		effectLabel,
		getPolicy,
		simulatePolicy,
		SURFACES,
		surfaceLabel,
		updatePolicy,
		validatePolicyCondition,
		type Effect,
		type Policy,
		type PolicyDraft,
		type PolicyMode,
		type SimulationResult,
		type Surface
	} from '$lib/services/provenance';
	import { profile, userDeviceSettings } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { formatLogTimestamp } from '$lib/time';
	import DecisionPill from './DecisionPill.svelte';
	import PolicyCheatSheet from './PolicyCheatSheet.svelte';
	import SurfaceBadge from './SurfaceBadge.svelte';
	import { Check, CircleAlert, Sparkles, X } from '@lucide/svelte';
	import { onMount, tick } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	// The Obot Policy editor: a CEL condition validated as it is typed, an effect, enforce or
	// shadow mode, the surfaces it covers, and the Supervisor's draft and simulate helpers.

	interface Props {
		/** The policy to edit; omit it to create one. */
		id?: string;
		/** Opens with the draft card focused, for "Draft with AI" from the list. */
		startWithDraft?: boolean;
		onTitle?: (title: string) => void;
		onSaved?: (policy: Policy, created: boolean) => void;
		onCancel: () => void;
	}

	let { id, startWithDraft = false, onTitle, onSaved, onCancel }: Props = $props();

	let loading = $state(true);
	let loadError = $state('');
	let saving = $state(false);
	let error = $state('');

	let builtIn = $state(false);
	let source = $state('');
	let name = $state('');
	let description = $state('');
	let condition = $state('');
	let policyEffect = $state<Effect>('deny');
	let mode = $state<PolicyMode>('shadow');
	let surfaces = $state<Surface[]>([]);
	let priority = $state(100);
	let guidance = $state('');
	let approversText = $state('');
	let approvalTTLMinutes = $state<number | undefined>();
	let enabled = $state(true);

	let validating = $state(false);
	let conditionError = $state('');
	let conditionChecked = $state(false);

	let draftText = $state('');
	let drafting = $state(false);
	let draftError = $state('');
	let draft = $state<PolicyDraft>();
	let draftInput = $state<HTMLTextAreaElement>();

	let simulateDays = $state(30);
	let simulating = $state(false);
	let simulateError = $state('');
	let simulation = $state<SimulationResult>();

	let readonly = $derived(profile.current.isAdminReadonly?.());
	let isNew = $derived(!id);
	let valid = $derived(
		name.trim() !== '' && condition.trim() !== '' && !conditionError && !validating
	);

	const effectOptions = EFFECTS.map((e) => ({ id: e, label: effectLabel(e) }));
	const conditionExample = '"untrusted" in session.labels && action.sink.external';
	const selectClass =
		'bg-base-200 dark:bg-base-200 dark:border-base-400 border border-transparent shadow-inner';

	function effectHelp(e: Effect) {
		switch (e) {
			case 'allow':
				return m.supervision_policy_effect_help_allow();
			case 'log':
				return m.supervision_policy_effect_help_log();
			case 'redact':
				return m.supervision_policy_effect_help_redact();
			case 'downshift':
				return m.supervision_policy_effect_help_downshift();
			case 'quarantine':
				return m.supervision_policy_effect_help_quarantine();
			case 'judge':
				return m.supervision_policy_effect_help_judge();
			case 'require_approval':
				return m.supervision_policy_effect_help_require_approval();
			case 'deny':
				return m.supervision_policy_effect_help_deny();
		}
	}

	function apply(p: Policy) {
		builtIn = !!p.builtIn;
		source = p.source ?? '';
		name = p.name ?? '';
		description = p.description ?? '';
		condition = p.condition ?? '';
		policyEffect = p.effect ?? 'deny';
		mode = p.mode === 'enforce' ? 'enforce' : 'shadow';
		surfaces = [...(p.surfaces ?? [])];
		priority = p.priority ?? 100;
		guidance = p.guidance ?? '';
		approversText = (p.approvers ?? []).join(', ');
		const ttl = approvalTTLSeconds(p);
		approvalTTLMinutes = ttl ? Math.round(ttl / 60) : undefined;
		enabled = p.enabled ?? true;
	}

	function build(): Policy {
		const approvers = approversText
			.split(',')
			.map((s) => s.trim())
			.filter(Boolean);
		return {
			...(id ? { id } : {}),
			name: name.trim(),
			description: description.trim() || undefined,
			condition: condition.trim(),
			effect: policyEffect,
			mode,
			priority: Number(priority) || 0,
			surfaces: surfaces.length ? surfaces : undefined,
			enabled,
			guidance: policyEffect === 'judge' && guidance.trim() ? guidance.trim() : undefined,
			approvers: policyEffect === 'require_approval' && approvers.length ? approvers : undefined,
			approvalTTL:
				policyEffect === 'require_approval'
					? approvalTTLFromSeconds(approvalTTLMinutes ? approvalTTLMinutes * 60 : undefined)
					: undefined,
			builtIn: builtIn || undefined,
			source: source || undefined
		};
	}

	onMount(async () => {
		if (id) {
			try {
				apply(await getPolicy(id, { dontLogErrors: true }));
			} catch (e) {
				loadError = parseErrorContent(e).message;
			} finally {
				loading = false;
			}
		} else {
			loading = false;
			if (startWithDraft) {
				await tick();
				draftInput?.focus();
			}
		}
	});

	$effect(() => {
		onTitle?.(isNew ? m.supervision_policy_new_title() : name || m.supervision_policy_edit_title());
	});

	// Validate the condition as it is typed, ignoring answers superseded by later edits.
	let validationSeq = 0;
	$effect(() => {
		const expr = condition.trim();
		const seq = ++validationSeq;
		if (!expr) {
			conditionError = '';
			conditionChecked = false;
			validating = false;
			return;
		}
		validating = true;
		conditionChecked = false;
		const timer = setTimeout(async () => {
			try {
				const result = await validatePolicyCondition(expr, { dontLogErrors: true });
				if (seq === validationSeq) {
					conditionError = result.valid ? '' : result.error || m.core_unknown_error_occurred();
					conditionChecked = true;
				}
			} catch (e) {
				if (seq === validationSeq) conditionError = parseErrorContent(e).message;
			} finally {
				if (seq === validationSeq) validating = false;
			}
		}, 400);
		return () => clearTimeout(timer);
	});

	function toggleSurface(s: Surface, checked: boolean) {
		surfaces = checked ? [...surfaces, s] : surfaces.filter((x) => x !== s);
	}

	async function save() {
		if (!valid || saving) return;
		saving = true;
		error = '';
		try {
			const body = build();
			const saved = id
				? await updatePolicy(id, body, { dontLogErrors: true })
				: await createPolicy(body, { dontLogErrors: true });
			success.add(m.supervision_policy_saved());
			onSaved?.(saved, !id);
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			saving = false;
		}
	}

	async function runDraft() {
		const text = draftText.trim();
		if (!text || drafting) return;
		drafting = true;
		draftError = '';
		draft = undefined;
		try {
			const result = await draftPolicy(text, { dontLogErrors: true });
			draft = result;
			// The draft fills the form but never changes identity fields of a saved policy.
			apply({ ...result.policy, builtIn, source: result.policy.source || text });
			success.add(m.supervision_policy_draft_applied());
		} catch (e) {
			const parsed = parseErrorContent(e);
			draftError =
				parsed.status === 503 ? m.supervision_policy_draft_unavailable() : parsed.message;
		} finally {
			drafting = false;
		}
	}

	async function runSimulation() {
		if (simulating || !condition.trim()) return;
		simulating = true;
		simulateError = '';
		try {
			simulation = await simulatePolicy(
				{ policy: { ...build(), name: name.trim() || 'draft' }, sinceHours: simulateDays * 24 },
				{ dontLogErrors: true }
			);
		} catch (e) {
			simulateError = parseErrorContent(e).message;
		} finally {
			simulating = false;
		}
	}

	let draftPassed = $derived(
		draft?.tests?.filter((t) => !t.error && t.expected === t.actual).length ?? 0
	);
</script>

{#if loading}
	<Skeleton type="card" class="min-h-96 w-full" />
{:else if loadError}
	<div class="notification-error rounded-md p-3 text-sm" role="alert">{loadError}</div>
{:else}
	<div class="flex flex-col gap-6 pb-4">
		{#if !readonly}
			<section class="paper gap-3" aria-label={m.supervision_policy_draft_title()}>
				<div class="flex flex-col">
					<h3 class="flex items-center gap-2 text-base font-semibold">
						<Sparkles class="text-primary size-4" />
						{m.supervision_policy_draft_title()}
					</h3>
					<p class="text-muted-content text-xs">{m.supervision_policy_draft_description()}</p>
				</div>
				<label for="policy-draft-text" class="sr-only">{m.supervision_policy_draft_label()}</label>
				<textarea
					id="policy-draft-text"
					bind:this={draftInput}
					class="text-input-filled min-h-20"
					placeholder={m.supervision_policy_draft_placeholder()}
					bind:value={draftText}></textarea>
				<div class="flex items-center justify-between gap-2">
					{#if draftError}
						<span class="text-error text-xs" role="alert">{draftError}</span>
					{:else}
						<span></span>
					{/if}
					<button
						type="button"
						class="btn btn-secondary btn-sm text-xs"
						disabled={drafting || !draftText.trim()}
						onclick={runDraft}
					>
						{#if drafting}<Loading
								class="size-4"
							/>{:else}{m.supervision_policy_draft_button()}{/if}
					</button>
				</div>
				{#if draft}
					<div class="flex flex-col gap-2 text-sm">
						<div class="flex flex-col gap-1">
							<span class="font-medium">{m.supervision_policy_draft_explanation()}</span>
							<p class="text-muted-content">{draft.explanation}</p>
						</div>
						{#if draft.tests?.length}
							<div class="flex items-baseline justify-between">
								<span class="font-medium">{m.supervision_policy_draft_tests()}</span>
								<span class="text-muted-content text-xs">
									{m.supervision_policy_draft_passed({
										passed: draftPassed,
										total: draft.tests.length
									})}
								</span>
							</div>
							<div class="overflow-x-auto">
								<table class="table table-sm">
									<thead>
										<tr>
											<th>{m.supervision_policy_draft_col_test()}</th>
											<th>{m.supervision_policy_draft_col_summary()}</th>
											<th>{m.supervision_policy_draft_col_expected()}</th>
											<th>{m.supervision_policy_draft_col_actual()}</th>
										</tr>
									</thead>
									<tbody>
										{#each draft.tests as t, i (`${t.name}-${i}`)}
											{@const pass = !t.error && t.expected === t.actual}
											<tr>
												<td class="font-medium">{t.name}</td>
												<td class="text-muted-content text-xs">{t.summary}</td>
												<td>
													{t.expected
														? m.supervision_policy_draft_match()
														: m.supervision_policy_draft_no_match()}
												</td>
												<td class="flex items-center gap-1">
													{#if pass}
														<Check class="text-success size-4" aria-label="pass" />
													{:else}
														<X class="text-error size-4" aria-label="fail" />
													{/if}
													{#if t.error}
														<span class="text-error text-xs">{t.error}</span>
													{:else}
														{t.actual
															? m.supervision_policy_draft_match()
															: m.supervision_policy_draft_no_match()}
													{/if}
												</td>
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
						{/if}
					</div>
				{/if}
			</section>
		{/if}

		<section class="paper gap-4">
			<div class="flex flex-wrap items-center justify-between gap-2">
				<div class="flex items-center gap-2">
					{#if builtIn}
						<span class="badge badge-sm badge-soft badge-primary">
							{m.supervision_policies_built_in()}
						</span>
					{/if}
					{#if source}
						<span class="text-muted-content text-xs" title={source}>
							{m.supervision_policy_source()}: "{source.length > 80
								? `${source.slice(0, 80)}…`
								: source}"
						</span>
					{/if}
				</div>
				<Toggle
					label={m.supervision_policy_enabled()}
					labelInline
					checked={enabled}
					disabled={readonly}
					onChange={(v) => (enabled = v)}
				/>
			</div>

			<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<label for="policy-name" class="text-sm font-light">{m.core_name()}</label>
					<input id="policy-name" class="text-input-filled" bind:value={name} disabled={readonly} />
				</div>
				<div class="flex flex-col gap-1">
					<label for="policy-description" class="text-sm font-light">{m.core_description()}</label>
					<input
						id="policy-description"
						class="text-input-filled"
						bind:value={description}
						disabled={readonly}
					/>
				</div>
			</div>

			<div class="flex flex-col gap-1">
				<label for="policy-condition" class="text-sm font-light"
					>{m.supervision_policy_condition()}</label
				>
				<p class="text-muted-content text-xs">
					{m.supervision_policy_condition_help()}
					<a href="https://cel.dev" target="_blank" rel="noopener noreferrer" class="text-link"
						>CEL</a
					>
				</p>
				<textarea
					id="policy-condition"
					class={twMerge(
						'text-input-filled min-h-24 font-mono text-sm',
						conditionError && 'border-error'
					)}
					placeholder={conditionExample}
					bind:value={condition}
					disabled={readonly}></textarea>
				{#if validating}
					<span class="text-muted-content text-xs"
						>{m.supervision_policy_condition_validating()}</span
					>
				{:else if conditionError}
					<span class="text-error flex items-center gap-1 text-xs" role="alert">
						<CircleAlert class="size-3" />
						{conditionError}
					</span>
				{:else if conditionChecked}
					<span class="text-success flex items-center gap-1 text-xs" role="status">
						<Check class="size-3" />
						{m.supervision_policy_condition_valid()}
					</span>
				{/if}
				<PolicyCheatSheet />
			</div>

			<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
				<div class="flex flex-col gap-1">
					<span id="policy-effect-label" class="text-sm font-light"
						>{m.supervision_policy_effect()}</span
					>
					<Select
						id="policy-effect"
						class={selectClass}
						ariaLabelledby="policy-effect-label"
						options={effectOptions}
						selected={policyEffect}
						onSelect={(o) => (policyEffect = o.id as Effect)}
						disabled={readonly}
					/>
					<span class="text-muted-content text-xs">{effectHelp(policyEffect)}</span>
				</div>

				<fieldset class="flex flex-col gap-1">
					<legend class="text-sm font-light">{m.supervision_policy_mode()}</legend>
					<div class="flex flex-col gap-2 pt-1">
						<label class="flex items-start gap-2 text-sm">
							<input
								type="radio"
								class="radio radio-sm mt-0.5"
								name="policy-mode"
								value="shadow"
								bind:group={mode}
								disabled={readonly}
							/>
							<span class="flex flex-col">
								<span>{m.supervision_mode_shadow()}</span>
								<span class="text-muted-content text-xs"
									>{m.supervision_policy_mode_shadow_help()}</span
								>
							</span>
						</label>
						<label class="flex items-start gap-2 text-sm">
							<input
								type="radio"
								class="radio radio-sm mt-0.5"
								name="policy-mode"
								value="enforce"
								bind:group={mode}
								disabled={readonly}
							/>
							<span class="flex flex-col">
								<span>{m.supervision_mode_enforce()}</span>
								<span class="text-muted-content text-xs"
									>{m.supervision_policy_mode_enforce_help()}</span
								>
							</span>
						</label>
					</div>
				</fieldset>
			</div>

			<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
				<fieldset class="flex flex-col gap-1">
					<legend class="text-sm font-light">{m.supervision_policy_surfaces()}</legend>
					<div class="flex flex-wrap gap-x-4 gap-y-1 pt-1">
						{#each SURFACES as s (s)}
							<label class="flex items-center gap-2 text-sm">
								<input
									type="checkbox"
									class="checkbox checkbox-sm"
									checked={surfaces.includes(s)}
									disabled={readonly}
									onchange={(e) => toggleSurface(s, (e.target as HTMLInputElement).checked)}
								/>
								{surfaceLabel(s)}
							</label>
						{/each}
					</div>
					<span class="text-muted-content text-xs">{m.supervision_policy_surfaces_help()}</span>
				</fieldset>

				<div class="flex flex-col gap-1">
					<label for="policy-priority" class="text-sm font-light"
						>{m.supervision_policy_priority()}</label
					>
					<input
						id="policy-priority"
						type="number"
						class="text-input-filled"
						bind:value={priority}
						disabled={readonly}
					/>
					<span class="text-muted-content text-xs">{m.supervision_policy_priority_help()}</span>
				</div>
			</div>

			{#if policyEffect === 'judge'}
				<div class="flex flex-col gap-1">
					<label for="policy-guidance" class="text-sm font-light"
						>{m.supervision_policy_guidance()}</label
					>
					<textarea
						id="policy-guidance"
						class="text-input-filled min-h-20"
						placeholder={m.supervision_policy_guidance_placeholder()}
						bind:value={guidance}
						disabled={readonly}></textarea>
					<span class="text-muted-content text-xs">{m.supervision_policy_guidance_help()}</span>
				</div>
			{/if}

			{#if policyEffect === 'require_approval'}
				<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
					<div class="flex flex-col gap-1">
						<label for="policy-approvers" class="text-sm font-light"
							>{m.supervision_policy_approvers()}</label
						>
						<input
							id="policy-approvers"
							class="text-input-filled"
							placeholder="admin, group:security"
							bind:value={approversText}
							disabled={readonly}
						/>
						<span class="text-muted-content text-xs">{m.supervision_policy_approvers_help()}</span>
					</div>
					<div class="flex flex-col gap-1">
						<label for="policy-approval-ttl" class="text-sm font-light"
							>{m.supervision_policy_approval_ttl()}</label
						>
						<input
							id="policy-approval-ttl"
							type="number"
							min="1"
							class="text-input-filled"
							bind:value={approvalTTLMinutes}
							disabled={readonly}
						/>
						<span class="text-muted-content text-xs"
							>{m.supervision_policy_approval_ttl_help()}</span
						>
					</div>
				</div>
			{/if}

			{#if error}
				<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
			{/if}
		</section>

		<section class="paper gap-3" aria-label={m.supervision_policy_simulate_title()}>
			<div class="flex flex-wrap items-end justify-between gap-3">
				<div class="flex flex-col">
					<h3 class="text-base font-semibold">{m.supervision_policy_simulate_title()}</h3>
					<p class="text-muted-content text-xs">{m.supervision_policy_simulate_description()}</p>
				</div>
				<div class="flex items-end gap-2">
					<div class="flex flex-col gap-1">
						<label for="policy-simulate-days" class="text-xs font-light"
							>{m.supervision_policy_simulate_days()}</label
						>
						<input
							id="policy-simulate-days"
							type="number"
							min="1"
							max="365"
							class="text-input-filled w-24"
							bind:value={simulateDays}
						/>
					</div>
					<button
						type="button"
						class="btn btn-secondary btn-sm text-xs"
						disabled={simulating || !condition.trim() || !!conditionError}
						onclick={runSimulation}
					>
						{#if simulating}<Loading
								class="size-4"
							/>{:else}{m.supervision_policy_simulate_button()}{/if}
					</button>
				</div>
			</div>
			{#if simulateError}
				<div class="notification-error rounded-md p-3 text-sm" role="alert">{simulateError}</div>
			{/if}
			{#if simulation}
				<div class="grid grid-cols-2 gap-2 md:grid-cols-6">
					{#each [{ label: m.supervision_policy_simulate_examined(), value: simulation.examined }, { label: m.supervision_policy_simulate_matched(), value: simulation.matched }, { label: m.supervision_policy_simulate_changed(), value: simulation.changed }, { label: m.supervision_policy_simulate_sessions(), value: simulation.sessions }, { label: m.supervision_policy_simulate_users(), value: simulation.users }] as tile (tile.label)}
						<div class="bg-base-200 dark:bg-base-300 flex flex-col rounded-md p-3">
							<span class="text-muted-content text-xs">{tile.label}</span>
							<span class="text-xl font-semibold tabular-nums">{tile.value}</span>
						</div>
					{/each}
					<div
						class="bg-base-200 dark:bg-base-300 flex flex-col rounded-md p-3"
						title={m.supervision_policy_simulate_fp_help()}
					>
						<span class="text-muted-content text-xs">{m.supervision_policy_simulate_fp_hint()}</span
						>
						<span class="text-xl font-semibold tabular-nums">
							{Math.round((simulation.falsePositiveHint ?? 0) * 100)}%
						</span>
					</div>
				</div>
				{#if simulation.errors > 0}
					<span class="text-warning text-xs">
						{m.supervision_policy_simulate_errors({ count: simulation.errors })}
					</span>
				{/if}
				<div class="flex flex-col gap-1">
					<span class="text-sm font-medium">{m.supervision_policy_simulate_samples()}</span>
					{#if !simulation.samples?.length}
						<span class="text-muted-content text-sm"
							>{m.supervision_policy_simulate_no_samples()}</span
						>
					{:else}
						<div class="overflow-x-auto">
							<table class="table table-sm">
								<thead>
									<tr>
										<th>{m.core_col_time()}</th>
										<th>{m.supervision_redteam_col_surface()}</th>
										<th>{m.supervision_col_summary()}</th>
										<th>{m.core_col_user()}</th>
										<th>{m.supervision_policy_simulate_col_recorded()}</th>
									</tr>
								</thead>
								<tbody>
									{#each simulation.samples as ev (ev.id)}
										<tr>
											<td class="font-mono text-xs whitespace-nowrap">
												{formatLogTimestamp(ev.at, userDeviceSettings.timeFormat)}
											</td>
											<td><SurfaceBadge surface={ev.surface} short /></td>
											<td class="text-xs">{ev.summary || describeAction(ev.action)}</td>
											<td class="text-xs">{ev.userID}</td>
											<td><DecisionPill decision={ev.decision} /></td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{/if}
				</div>
			{/if}
		</section>
	</div>

	{#if !readonly}
		<div
			class="bg-base-200 dark:bg-base-100 sticky bottom-0 left-0 z-40 flex w-full justify-end gap-2 py-4"
		>
			<button type="button" class="btn btn-secondary text-sm" onclick={onCancel}
				>{m.common_cancel()}</button
			>
			<button
				type="button"
				class="btn btn-primary text-sm"
				disabled={!valid || saving}
				onclick={save}
			>
				{#if saving}<Loading class="size-4" />{:else}{m.core_save()}{/if}
			</button>
		</div>
	{/if}
{/if}
