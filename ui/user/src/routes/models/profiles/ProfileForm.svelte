<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import SearchModels from '$lib/components/admin/SearchModels.svelte';
	import SearchUsers from '$lib/components/admin/SearchUsers.svelte';
	import SubjectName from '$lib/components/admin/SubjectName.svelte';
	import BudgetEditor from '$lib/components/llm-gateway/BudgetEditor.svelte';
	import GuardrailPolicyEditor from '$lib/components/llm-gateway/GuardrailPolicyEditor.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { CommonModelProviderIds } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		matchesModelPattern,
		ModelAlias,
		ModelAliasLabels,
		ModelUsage,
		ModelUsageLabels,
		type DefaultModelAlias,
		type Model,
		type ModelAccessPolicy,
		type ModelAccessPolicyManifest,
		type ModelResource,
		type OrgGroup,
		type OrgUser
	} from '$lib/services';
	import {
		createBudget,
		deleteBudget,
		formatMetric,
		listBudgets,
		METRIC_SUMMARY_LABELS,
		updateBudget,
		windowLabel,
		type Budget
	} from '$lib/services/llm-gateway/gateway';
	import {
		ACTION_LABELS,
		APPLY_TO_LABELS,
		createGuardrailPolicy,
		deleteGuardrailPolicy,
		listGuardrailPolicies,
		listGuardrails,
		updateGuardrailPolicy,
		type Guardrail,
		type GuardrailPolicy
	} from '$lib/services/llm-gateway/guardrails';
	import {
		PROFILES_URL,
		profileGrantedModels,
		profileUrl
	} from '$lib/services/llm-gateway/profiles';
	import { accessibleModels, defaultModelAliases as defaultModelAliasesStore } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import {
		convertSubjectsToTableData,
		resolveSubjectFromGroup,
		resolveSubjectPickerById,
		resolveSubjects
	} from '$lib/subjectResolver';
	import { goto, replaceState } from '$lib/url';
	import { ArrowLeft, Plus, Trash2, TriangleAlert, UsersRound, X } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';

	// One form for a gateway profile: its users and groups, its models, its budgets and its applied
	// guardrails (guardrail policies). Budgets and applied guardrails are edited in dialogs and kept
	// in the form's draft; Save writes the profile first, then each of them.
	interface Props {
		modelAccessPolicy?: ModelAccessPolicy;
		readonly?: boolean;
	}

	let { modelAccessPolicy: initial, readonly = false }: Props = $props();

	// A budget or applied guardrail in the draft. `dirty` marks one changed since it was saved;
	// one without an ID is new.
	type Item<T> = { key: string; value: T; dirty: boolean };

	const llmModelWarning = m.models_access_policies_only_llm_models_allowed();
	let nextKey = 0;
	const newKey = () => `draft-${nextKey++}`;

	function profileJson(p: Pick<ModelAccessPolicyManifest, 'displayName' | 'subjects' | 'models'>) {
		return JSON.stringify({
			displayName: p.displayName.trim(),
			subjects: p.subjects ?? [],
			models: p.models ?? []
		});
	}

	let draft = $state<ModelAccessPolicyManifest>(
		untrack(() =>
			initial
				? structuredClone($state.snapshot(initial))
				: { displayName: '', subjects: [], models: [] }
		)
	);
	// The profile's ID once it exists, and what was last saved of it.
	let profileID = $state(untrack(() => initial?.id));
	let savedJson = $state(untrack(() => (initial ? profileJson(initial) : '')));
	let savedModelsJson = $state(untrack(() => JSON.stringify(initial?.models ?? [])));

	let budgets = $state<Item<Budget>[]>([]);
	let policies = $state<Item<GuardrailPolicy>[]>([]);
	let removedBudgets = $state<Budget[]>([]);
	let removedPolicies = $state<GuardrailPolicy[]>([]);
	let loadingItems = $state(untrack(() => !!initial?.id));

	let models = $state<Model[]>([]);
	let loadingModels = $state(true);
	let aliases = $state<DefaultModelAlias[]>(defaultModelAliasesStore.current);
	let guardrails = $state<Guardrail[]>([]);
	let usersAndGroups = $state<{ users: OrgUser[]; groups: OrgGroup[] }>();
	let loadingUsersAndGroups = $state(false);

	let saving = $state(false);
	let error = $state('');
	let failures = $state<string[]>([]);
	let deleting = $state(false);

	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();
	let addModelDialog = $state<ReturnType<typeof SearchModels>>();
	let budgetDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let policyDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	// The item a dialog edits: a key from the draft, 'new', or undefined when closed.
	let editingBudget = $state<string>();
	let editingPolicy = $state<string>();

	onMount(async () => {
		const id = profileID;
		const [fetchedModels, fetchedAliases, fetchedGuardrails, fetchedBudgets, fetchedPolicies] =
			await Promise.all([
				AdminService.listModels({ all: true }).catch(() => [] as Model[]),
				UserService.listDefaultModelAliases().catch(() => aliases),
				listGuardrails().catch(() => [] as Guardrail[]),
				id ? listBudgets({ profileID: id }).catch(() => [] as Budget[]) : [],
				id ? listGuardrailPolicies({ profileID: id }).catch(() => [] as GuardrailPolicy[]) : []
			]);
		models = fetchedModels;
		aliases = fetchedAliases;
		guardrails = fetchedGuardrails;
		loadingModels = false;
		budgets = fetchedBudgets.map((value) => ({ key: newKey(), value, dirty: false }));
		policies = fetchedPolicies
			.filter((p) => p.target === 'llm')
			.map((value) => ({ key: newKey(), value, dirty: false }));
		loadingItems = false;
	});

	$effect(() => {
		// Groups are resolved by ID, not listed: the directory can hold tens of thousands of them and
		// only the ones attached here are needed.
		const subjects = draft.subjects ?? [];
		if (subjects.length === 0) {
			loadingUsersAndGroups = false;
			return;
		}
		loadingUsersAndGroups = !untrack(() => usersAndGroups);
		const controller = new AbortController();
		resolveSubjects(
			subjects,
			untrack(() => usersAndGroups),
			{ signal: controller.signal }
		)
			.then((resolved) => {
				if (controller.signal.aborted) return;
				usersAndGroups = resolved;
				loadingUsersAndGroups = false;
			})
			.catch((err) => {
				if (controller.signal.aborted) return;
				console.error('Failed to load users and groups:', err);
				loadingUsersAndGroups = false;
			});
		return () => controller.abort();
	});

	let llmModels = $derived(models.filter((model) => model.usage === ModelUsage.LLM));
	let modelsMap = $derived(new Map(models.map((model) => [model.id, model])));
	let aliasToModel = $derived(
		new Map(aliases.map((alias) => [alias.alias, modelsMap.get(alias.model)]))
	);

	let subjectRows = $derived(
		convertSubjectsToTableData(
			draft.subjects ?? [],
			usersAndGroups?.users ?? [],
			usersAndGroups?.groups ?? []
		)
	);

	type ModelRow = {
		id: string;
		name: string;
		detail?: string;
		// The model an alias points to.
		effective?: string;
		isAlias?: boolean;
		isConfigured?: boolean;
		isPattern?: boolean;
		matchCount?: number;
		warning?: string;
	};

	let modelRows = $derived(
		(draft.models ?? []).map((resource: ModelResource): ModelRow => {
			if (resource.id.startsWith('obot://')) {
				const aliasName = resource.id.slice('obot://'.length);
				const model = aliasToModel.get(aliasName as ModelAlias);
				const allowed = aliasName === ModelAlias.Llm || aliasName === ModelAlias.LlmMini;
				return {
					id: resource.id,
					name: aliasName,
					detail: ModelAliasLabels[aliasName as ModelAlias] || aliasName,
					effective: model ? model.displayName || model.targetModel : m.models_not_configured(),
					isAlias: true,
					isConfigured: !!model,
					warning: allowed ? undefined : llmModelWarning
				};
			}
			if (resource.id === '*') {
				return { id: resource.id, name: m.models_all_models() };
			}
			if (resource.id.endsWith('*')) {
				// Wildcard suffix pattern; count the currently matching models (case-sensitive prefix
				// match on target model, like the backend; a pattern without a provider also matches
				// the part after it).
				const prefix = resource.id.slice(0, -1);
				return {
					id: resource.id,
					name: resource.id,
					isPattern: true,
					matchCount: models.filter((c) => matchesModelPattern(prefix, c.targetModel)).length
				};
			}
			const found = modelsMap.get(resource.id);
			let warning: string | undefined;
			if (!loadingModels && !found) {
				warning = m.models_access_policies_model_no_longer_exists();
			} else if (found && found.usage !== ModelUsage.LLM && found.usage !== ModelAlias.LlmMini) {
				warning = llmModelWarning;
			}
			return {
				id: resource.id,
				name: found?.displayName || found?.name || resource.id,
				detail: found?.usage
					? ModelUsageLabels[found.usage as ModelUsage] || found.usage
					: undefined,
				warning
			};
		})
	);
	let invalidModelCount = $derived(modelRows.filter((row) => row.warning).length);

	// A budget covers only its profile's models: those the draft grants now.
	let gatewayModels = $derived(
		models.filter((model) => model.modelProvider === CommonModelProviderIds.LLM_GATEWAY)
	);
	let grantedModels = $derived(profileGrantedModels(draft, gatewayModels, aliases));
	let grantedIDs = $derived(new Set(grantedModels.map((model) => model.id)));
	let profileModelsChanged = $derived(JSON.stringify(draft.models ?? []) !== savedModelsJson);

	// The models a budget keeps once those the profile no longer grants are dropped, or undefined
	// when they stay as they are.
	function prunedModels(item: Item<Budget>): string[] | undefined {
		const chosen = item.value.models ?? [];
		if (!chosen.length || loadingModels || !(item.dirty || profileModelsChanged)) return undefined;
		const kept = chosen.filter((id) => grantedIDs.has(id));
		return kept.length === chosen.length ? undefined : kept;
	}

	// Budgets limited to specific models, none of which the profile grants anymore. Saving them
	// would make them cover all of the profile's models, so they must be edited first.
	let budgetsWithoutModels = $derived(
		new Set(budgets.filter((item) => prunedModels(item)?.length === 0).map((item) => item.key))
	);

	let guardrailNames = $derived(Object.fromEntries(guardrails.map((g) => [g.id, g.name])));

	function budgetSummary(item: Item<Budget>) {
		const b = item.value;
		const limits = b.limits
			.map((l) =>
				m.models_budgets_limit_summary({
					amount: formatMetric(l.metric, l.amount),
					metric: METRIC_SUMMARY_LABELS[l.metric],
					window: windowLabel(l)
				})
			)
			.join('; ');
		const scope = b.perUser ? m.models_budgets_each_user() : m.models_budgets_shared();
		// Count the models the budget keeps once those the profile no longer grants are dropped,
		// as Save will.
		const pruned = prunedModels(item);
		const chosen = pruned ?? b.models ?? [];
		const covered =
			chosen.length || pruned
				? chosen.length === 1
					? m.models_budgets_models_count_one({ count: chosen.length })
					: m.models_budgets_models_count_other({ count: chosen.length })
				: m.models_budgets_all_models();
		return [scope, limits, covered].filter(Boolean).join(' · ');
	}

	function policySummary(p: GuardrailPolicy) {
		return [
			APPLY_TO_LABELS[p.applyTo] ?? p.applyTo,
			ACTION_LABELS[p.action] ?? p.action,
			(p.guardrailIDs ?? []).map((id) => guardrailNames[id] ?? id).join(', ')
		]
			.filter(Boolean)
			.join(' · ');
	}

	let profileChanged = $derived(!profileID || profileJson(draft) !== savedJson);
	let hasChanges = $derived(
		profileChanged ||
			removedBudgets.length > 0 ||
			removedPolicies.length > 0 ||
			budgets.some((item) => item.dirty || !item.value.id || prunedModels(item)) ||
			policies.some((item) => item.dirty || !item.value.id)
	);
	let valid = $derived(
		draft.displayName.trim().length > 0 &&
			(draft.subjects?.length ?? 0) > 0 &&
			invalidModelCount === 0 &&
			budgetsWithoutModels.size === 0
	);

	function errorMessage(e: unknown) {
		return parseErrorContent(e).message;
	}

	async function save() {
		saving = true;
		error = '';
		failures = [];
		try {
			// The profile first: its budgets and applied guardrails belong to it.
			let id = profileID;
			try {
				if (!id) {
					const created = await AdminService.createModelAccessPolicy(draft);
					id = created.id;
					draft = { ...draft, ...created };
					profileID = id;
					// The profile exists now; if anything after this fails, the form stays on it.
					replaceState(profileUrl(id), {});
				} else if (profileChanged) {
					const updated = await AdminService.updateModelAccessPolicy(id, draft);
					draft = { ...draft, ...updated };
				}
				savedJson = profileJson(draft);
			} catch (e) {
				error = errorMessage(e);
				return;
			}

			const failed: string[] = [];
			const record = (name: string, e: unknown) =>
				failed.push(m.models_profile_save_item_failed({ name, error: errorMessage(e) }));

			for (const budget of removedBudgets) {
				try {
					if (budget.id) await deleteBudget(budget.id, { dontLogErrors: true });
					removedBudgets = removedBudgets.filter((b) => b !== budget);
				} catch (e) {
					record(budget.name, e);
				}
			}
			for (const policy of removedPolicies) {
				try {
					if (policy.id) await deleteGuardrailPolicy(policy.id, { dontLogErrors: true });
					removedPolicies = removedPolicies.filter((p) => p !== policy);
				} catch (e) {
					record(policy.name, e);
				}
			}
			for (const item of budgets) {
				const pruned = prunedModels(item);
				if (item.value.id && !item.dirty && !pruned) continue;
				const body: Budget = { ...item.value, profileID: id, models: pruned ?? item.value.models };
				try {
					const saved = body.id
						? await updateBudget(body.id, body, { dontLogErrors: true })
						: await createBudget(body, { dontLogErrors: true });
					item.value = { ...body, ...saved };
					item.dirty = false;
				} catch (e) {
					record(body.name, e);
				}
			}
			for (const item of policies) {
				if (item.value.id && !item.dirty) continue;
				const body: GuardrailPolicy = { ...item.value, profileID: id };
				try {
					const saved = body.id
						? await updateGuardrailPolicy(body.id, body, { dontLogErrors: true })
						: await createGuardrailPolicy(body, { dontLogErrors: true });
					item.value = { ...body, ...saved };
					item.dirty = false;
				} catch (e) {
					record(body.name, e);
				}
			}
			savedModelsJson = JSON.stringify(draft.models ?? []);
			accessibleModels.refresh().catch(() => undefined);

			if (failed.length) {
				// What failed stays in the draft, so saving again retries just that.
				error = m.models_profile_save_partial();
				failures = failed;
				return;
			}
			success.add(m.models_profile_saved());
			goto(PROFILES_URL);
		} finally {
			saving = false;
		}
	}

	function openBudget(key: string) {
		editingBudget = key;
		budgetDialog?.open();
	}

	function openPolicy(key: string) {
		editingPolicy = key;
		policyDialog?.open();
	}

	function closeBudget() {
		budgetDialog?.close();
		editingBudget = undefined;
	}

	function closePolicy() {
		policyDialog?.close();
		editingPolicy = undefined;
	}

	function removeBudget(item: Item<Budget>) {
		budgets = budgets.filter((b) => b.key !== item.key);
		if (item.value.id) removedBudgets = [...removedBudgets, item.value];
	}

	function removePolicy(item: Item<GuardrailPolicy>) {
		policies = policies.filter((p) => p.key !== item.key);
		if (item.value.id) removedPolicies = [...removedPolicies, item.value];
	}

	let budgetBeingEdited = $derived(budgets.find((b) => b.key === editingBudget));
	let policyBeingEdited = $derived(policies.find((p) => p.key === editingPolicy));

	let title = $derived(
		!profileID
			? m.models_profiles_new()
			: readonly
				? m.models_profile_view()
				: m.models_profile_edit()
	);
</script>

<form
	class="mx-auto flex w-full max-w-4xl flex-col gap-3 rounded-xl border border-base-300 bg-base-100 p-5 shadow-sm dark:border-base-400 dark:bg-base-300"
	onsubmit={(e) => {
		e.preventDefault();
		if (!readonly && valid && hasChanges && !saving) void save();
	}}
>
	<header class="flex items-start gap-3">
		<IconButton
			disabled={saving}
			tooltip={{ text: m.models_profile_back() }}
			onclick={() => goto(PROFILES_URL)}
		>
			<ArrowLeft class="size-4" />
		</IconButton>
		<div class="min-w-0 grow">
			<h1 class="text-md font-semibold">{title}</h1>
			<p class="text-muted-content text-sm">{m.models_profile_form_description()}</p>
		</div>
		{#if profileID && !readonly}
			<IconButton
				variant="danger"
				disabled={saving}
				tooltip={{ text: m.models_delete_profile() }}
				onclick={() => (deleting = true)}
			>
				<Trash2 class="size-4" />
			</IconButton>
		{/if}
	</header>
	<div class="divider my-0"></div>

	<label class="flex flex-col gap-1 text-sm font-light" for="profile-name">
		{m.core_name()}
		<input
			id="profile-name"
			class="text-input-filled w-full"
			bind:value={draft.displayName}
			disabled={readonly || saving}
		/>
	</label>

	<section>
		<div class="divider mt-5 mb-3 text-sm font-semibold">{m.core_users_and_groups()}</div>
		<div class="mb-3 flex items-start justify-between gap-3">
			<p class="text-muted-content text-sm font-light">{m.models_profile_users_description()}</p>
			{#if !readonly}
				<button
					type="button"
					class="btn btn-secondary btn-sm flex shrink-0 items-center gap-1 text-xs"
					disabled={loadingUsersAndGroups || saving}
					onclick={() => addUserGroupDialog?.open()}
				>
					<Plus class="size-4" />
					{m.core_add_user_group()}
				</button>
			{/if}
		</div>
		{#if loadingUsersAndGroups}
			<div class="my-2 flex items-center justify-center"><Loading class="size-6" /></div>
		{:else if subjectRows.length === 0}
			<p class="text-muted-content pt-1 pb-3 text-center text-xs font-light italic">
				{m.core_no_users_or_groups_added()}
			</p>
		{:else}
			<ul class="flex flex-col">
				{#each subjectRows as row, index (row.id)}
					<li class="flex items-center justify-between gap-2 p-2">
						<div class="flex min-w-0 items-center gap-2">
							<div
								class="bg-base-300 dark:bg-base-200 flex size-8 shrink-0 items-center justify-center rounded-full"
							>
								<UsersRound class="size-4" />
							</div>
							<div class="flex min-w-0 flex-col">
								<SubjectName
									class="truncate text-sm font-light"
									name={row.displayName}
									disabled={row.disabled}
								/>
								<span class="text-muted-content text-xs">{row.type}</span>
							</div>
						</div>
						{#if !readonly}
							<IconButton
								class="size-8"
								variant="danger"
								disabled={saving}
								tooltip={{ text: m.core_delete_user_group() }}
								onclick={() => {
									draft.subjects = draft.subjects?.filter(
										(subject) => resolveSubjectPickerById(subject) !== row.id
									);
								}}
							>
								<X class="size-4" />
							</IconButton>
						{/if}
					</li>
					{#if index < subjectRows.length - 1}
						<li class="divider my-0 px-2 before:h-px after:h-px" aria-hidden="true"></li>
					{/if}
				{/each}
			</ul>
		{/if}
	</section>

	<section>
		<div class="divider mt-5 mb-3 text-sm font-semibold">{m.models_title()}</div>
		<div class="mb-3 flex items-start justify-between gap-3">
			<p class="text-muted-content text-sm font-light">{m.models_profile_models_description()}</p>
			{#if !readonly}
				<button
					type="button"
					class="btn btn-secondary btn-sm flex shrink-0 items-center gap-1 text-xs"
					disabled={saving}
					onclick={() => addModelDialog?.open()}
				>
					<Plus class="size-4" />
					{m.models_access_policies_add_model()}
				</button>
			{/if}
		</div>
		{#if invalidModelCount > 0}
			<div class="notification-alert mb-2 flex items-start gap-1" role="alert">
				<TriangleAlert class="text-warning size-4 shrink-0" />
				<p class="text-xs">{m.models_access_policies_invalid_models_notice()}</p>
			</div>
		{/if}
		{#if loadingModels && modelRows.length > 0}
			<div class="my-2 flex items-center justify-center"><Loading class="size-6" /></div>
		{:else if modelRows.length === 0}
			<p class="text-muted-content pt-1 pb-3 text-center text-xs font-light italic">
				{m.models_profile_no_models_added()}
			</p>
		{:else}
			<ul class="flex flex-col">
				{#each modelRows as row, index (row.id)}
					<li class="flex items-center justify-between gap-2 p-2">
						<div class="flex min-w-0 flex-col">
							<div class="flex items-center gap-2">
								<span class={row.isPattern ? 'font-mono text-sm' : 'text-sm'}>{row.name}</span>
								{#if row.isAlias}
									<span class="text-muted-content text-xs" class:text-warning={!row.isConfigured}
										>{row.effective}</span
									>
								{/if}
								{#if row.isPattern}
									<span
										class="bg-base-300 dark:bg-base-400 rounded-full px-2 py-0.5 text-xs font-medium"
									>
										{m.models_access_policies_pattern()}
									</span>
								{/if}
							</div>
							{#if row.isPattern}
								<span class="text-muted-content text-xs">
									{#if (row.matchCount ?? 0) > 0}
										{row.matchCount === 1
											? m.models_access_policies_pattern_matches_one({ count: 1 })
											: m.models_access_policies_pattern_matches_other({
													count: row.matchCount ?? 0
												})}
									{:else}
										{m.models_access_policies_pattern_no_matches()}
									{/if}
								</span>
							{:else if row.detail}
								<span class="text-muted-content text-xs">{row.detail}</span>
							{/if}
							{#if row.warning}
								<span class="text-warning flex items-center gap-1 text-xs">
									<TriangleAlert class="size-3 shrink-0" />
									{row.warning}
								</span>
							{/if}
						</div>
						{#if !readonly}
							<IconButton
								class="size-8"
								variant="danger"
								disabled={saving}
								tooltip={{ text: m.models_access_policies_remove_model() }}
								onclick={() => {
									draft.models = draft.models?.filter((model) => model.id !== row.id) ?? [];
								}}
							>
								<X class="size-4" />
							</IconButton>
						{/if}
					</li>
					{#if index < modelRows.length - 1}
						<li class="divider my-0 px-2 before:h-px after:h-px" aria-hidden="true"></li>
					{/if}
				{/each}
			</ul>
		{/if}
	</section>

	<section>
		<div class="divider mt-5 mb-3 text-sm font-semibold">{m.models_profiles_budgets()}</div>
		<div class="mb-3 flex items-start justify-between gap-3">
			<p class="text-muted-content text-sm font-light">
				{m.models_profiles_budgets_description()}
			</p>
			{#if !readonly}
				<button
					type="button"
					class="btn btn-secondary btn-sm flex shrink-0 items-center gap-1 text-xs"
					disabled={saving}
					onclick={() => openBudget('new')}
				>
					<Plus class="size-4" />
					{m.models_budgets_add()}
				</button>
			{/if}
		</div>
		{#if budgets.length > 0 && !draft.models?.length}
			<div class="notification-alert mb-2 flex items-center gap-2 p-3 text-sm">
				<TriangleAlert class="size-4 shrink-0" />
				{m.models_profiles_budgets_no_models()}
			</div>
		{/if}
		{#if loadingItems}
			<div class="my-2 flex items-center justify-center"><Loading class="size-6" /></div>
		{:else if budgets.length === 0}
			<p class="text-muted-content pt-1 pb-3 text-center text-xs font-light italic">
				{m.models_profiles_budgets_empty()}
			</p>
		{:else}
			<ul class="flex flex-col">
				{#each budgets as item, index (item.key)}
					<li class="flex items-center justify-between gap-2 rounded-md p-2 hover:bg-base-200">
						<div class="flex min-w-0 grow flex-col">
							<button
								type="button"
								class="flex min-w-0 grow flex-col text-left"
								aria-label={m.models_profiles_edit_item({ name: item.value.name })}
								onclick={() => openBudget(item.key)}
							>
								<span class="flex items-center gap-2 text-sm">
									<span class="truncate">{item.value.name}</span>
									{#if !item.value.enabled}
										<span class="badge badge-sm">{m.core_status_disabled()}</span>
									{/if}
									{#if !item.value.id || item.dirty}
										<span class="badge badge-sm badge-outline"
											>{m.models_profile_item_unsaved()}</span
										>
									{/if}
								</span>
								<span class="text-muted-content truncate text-xs">{budgetSummary(item)}</span>
							</button>
							{#if budgetsWithoutModels.has(item.key)}
								<span role="alert" class="text-warning flex items-center gap-1 text-xs">
									<TriangleAlert class="size-3 shrink-0" />
									{m.models_budgets_no_granted_models()}
								</span>
							{/if}
						</div>
						{#if !readonly}
							<IconButton
								class="size-8"
								variant="danger"
								disabled={saving}
								tooltip={{ text: m.models_budgets_delete() }}
								onclick={() => removeBudget(item)}
							>
								<X class="size-4" />
							</IconButton>
						{/if}
					</li>
					{#if index < budgets.length - 1}
						<li class="divider my-0 px-2 before:h-px after:h-px" aria-hidden="true"></li>
					{/if}
				{/each}
			</ul>
		{/if}
	</section>

	<section>
		<div class="divider mt-5 mb-3 text-sm font-semibold">
			{m.models_profiles_applied_guardrails()}
		</div>
		<div class="mb-3 flex items-start justify-between gap-3">
			<p class="text-muted-content text-sm font-light">
				{m.models_profiles_applied_guardrails_description()}
			</p>
			{#if !readonly}
				<button
					type="button"
					class="btn btn-secondary btn-sm flex shrink-0 items-center gap-1 text-xs"
					disabled={saving}
					onclick={() => openPolicy('new')}
				>
					<Plus class="size-4" />
					{m.models_profiles_add_guardrail()}
				</button>
			{/if}
		</div>
		{#if loadingItems}
			<div class="my-2 flex items-center justify-center"><Loading class="size-6" /></div>
		{:else if policies.length === 0}
			<p class="text-muted-content pt-1 pb-3 text-center text-xs font-light italic">
				{m.models_profiles_applied_guardrails_empty()}
			</p>
		{:else}
			<ul class="flex flex-col">
				{#each policies as item, index (item.key)}
					<li class="flex items-center justify-between gap-2 rounded-md p-2 hover:bg-base-200">
						<button
							type="button"
							class="flex min-w-0 grow flex-col text-left"
							aria-label={m.models_profiles_edit_item({ name: item.value.name })}
							onclick={() => openPolicy(item.key)}
						>
							<span class="flex items-center gap-2 text-sm">
								<span class="truncate">{item.value.name}</span>
								{#if !item.value.enabled}
									<span class="badge badge-sm">{m.core_status_disabled()}</span>
								{/if}
								{#if !item.value.id || item.dirty}
									<span class="badge badge-sm badge-outline">{m.models_profile_item_unsaved()}</span
									>
								{/if}
							</span>
							<span class="text-muted-content truncate text-xs">{policySummary(item.value)}</span>
						</button>
						{#if !readonly}
							<IconButton
								class="size-8"
								variant="danger"
								disabled={saving}
								tooltip={{ text: m.models_applied_guardrail_remove() }}
								onclick={() => removePolicy(item)}
							>
								<X class="size-4" />
							</IconButton>
						{/if}
					</li>
					{#if index < policies.length - 1}
						<li class="divider my-0 px-2 before:h-px after:h-px" aria-hidden="true"></li>
					{/if}
				{/each}
			</ul>
		{/if}
	</section>

	{#if error}
		<div class="notification-error flex flex-col gap-1 rounded-md p-3 text-sm" role="alert">
			<p>{error}</p>
			{#if failures.length}
				<ul class="list-disc pl-5 text-xs">
					{#each failures as failure, i (i)}
						<li>{failure}</li>
					{/each}
				</ul>
			{/if}
		</div>
	{/if}

	{#if !readonly}
		<div class="flex justify-end gap-2 pt-2">
			<button
				type="button"
				class="btn btn-ghost text-sm"
				disabled={saving}
				onclick={() => goto(PROFILES_URL)}
			>
				{m.common_cancel()}
			</button>
			<button
				type="submit"
				class="btn btn-primary text-sm"
				disabled={!valid || !hasChanges || saving}
			>
				{#if saving}<Loading class="size-4" />{:else}{m.core_save()}{/if}
			</button>
		</div>
	{/if}
</form>

<ResponsiveDialog
	bind:this={budgetDialog}
	title={editingBudget === 'new'
		? m.models_budgets_add()
		: readonly
			? m.models_budgets_fallback_title()
			: m.models_budgets_edit()}
	class="max-w-3xl"
	onClose={() => (editingBudget = undefined)}
>
	<div class="max-h-[75vh] overflow-y-auto pt-4">
		{#if editingBudget}
			{#key editingBudget}
				<BudgetEditor
					budget={budgetBeingEdited?.value}
					models={grantedModels}
					{readonly}
					onCancel={closeBudget}
					onSave={(value) => {
						if (budgetBeingEdited) {
							budgetBeingEdited.value = value;
							budgetBeingEdited.dirty = true;
						} else {
							budgets = [...budgets, { key: newKey(), value, dirty: true }];
						}
						closeBudget();
					}}
				/>
			{/key}
		{/if}
	</div>
</ResponsiveDialog>

<ResponsiveDialog
	bind:this={policyDialog}
	title={editingPolicy === 'new'
		? m.models_profiles_add_guardrail()
		: readonly
			? m.models_applied_guardrail()
			: m.models_applied_guardrail_edit()}
	class="max-w-3xl"
	onClose={() => (editingPolicy = undefined)}
>
	<div class="max-h-[75vh] overflow-y-auto pt-4">
		{#if editingPolicy}
			{#key editingPolicy}
				<GuardrailPolicyEditor
					policy={policyBeingEdited?.value}
					{readonly}
					onCancel={closePolicy}
					onSave={(value, known) => {
						guardrails = known;
						if (policyBeingEdited) {
							policyBeingEdited.value = value;
							policyBeingEdited.dirty = true;
						} else {
							policies = [...policies, { key: newKey(), value, dirty: true }];
						}
						closePolicy();
					}}
				/>
			{/key}
		{/if}
	</div>
</ResponsiveDialog>

<SearchUsers
	bind:this={addUserGroupDialog}
	filterIds={draft.subjects?.map(resolveSubjectPickerById) ?? []}
	onAdd={async (users: OrgUser[], groups: OrgGroup[]) => {
		const existing = new Set(draft.subjects?.map(resolveSubjectPickerById) ?? []);
		const added = [
			...users
				.filter((user) => !existing.has(user.id))
				.map((user) => ({ type: 'user' as const, id: user.id })),
			...groups.filter((group) => !existing.has(group.id)).map((g) => resolveSubjectFromGroup(g))
		];
		draft.subjects = [...(draft.subjects ?? []), ...added];
	}}
/>

<SearchModels
	bind:this={addModelDialog}
	models={llmModels}
	defaultAliases={aliases}
	exclude={draft.models?.map((model) => model.id) ?? []}
	onAdd={async (ids: string[]) => {
		const existing = new Set(draft.models?.map((model) => model.id) ?? []);
		draft.models = [
			...(draft.models ?? []),
			...ids.filter((id) => !existing.has(id)).map((id) => ({ id }))
		];
	}}
/>

<Confirm
	msg={draft.displayName
		? m.models_delete_profile_named({ name: draft.displayName })
		: m.models_delete_this_profile()}
	show={deleting}
	loading={saving}
	onsuccess={async () => {
		if (!profileID) return;
		saving = true;
		try {
			await AdminService.deleteModelAccessPolicy(profileID);
			accessibleModels.refresh().catch(() => undefined);
			goto(PROFILES_URL);
		} catch (e) {
			error = errorMessage(e);
			deleting = false;
		} finally {
			saving = false;
		}
	}}
	oncancel={() => (deleting = false)}
/>
