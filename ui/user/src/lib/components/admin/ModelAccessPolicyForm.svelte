<script lang="ts">
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type ModelAccessPolicy,
		type ModelAccessPolicyManifest,
		type ModelResource,
		type OrgUser,
		type OrgGroup,
		ModelUsage,
		ModelUsageLabels,
		ModelAlias,
		ModelAliasLabels,
		type Model
	} from '$lib/services';
	import { defaultModelAliases as defaultModelAliasesStore } from '$lib/stores';
	import { goto } from '$lib/url';
	import {
		convertSubjectsToTableData,
		resolveSubjects,
		resolveSubjectFromGroup,
		resolveSubjectPickerById
	} from '../../subjectResolver';
	import Confirm from '../Confirm.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import SearchModels from './SearchModels.svelte';
	import SearchUsers from './SearchUsers.svelte';
	import { Plus, Trash2, TriangleAlert } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		modelAccessPolicy?: ModelAccessPolicy;
		onCreate?: (modelAccessPolicy: ModelAccessPolicy) => void;
		onUpdate?: (modelAccessPolicy: ModelAccessPolicy) => void;
		readonly?: boolean;
	}

	let {
		modelAccessPolicy: initialModelAccessPolicy,
		onCreate,
		onUpdate,
		readonly
	}: Props = $props();

	const duration = PAGE_TRANSITION_DURATION;
	const llmModelWarning = m.models_access_policies_only_llm_models_allowed();
	let modelAccessPolicy = $state(
		untrack(
			() =>
				initialModelAccessPolicy ??
				({
					displayName: '',
					subjects: [],
					models: []
				} as ModelAccessPolicyManifest)
		)
	);

	let saving = $state<boolean | undefined>();
	let usersAndGroups = $state<{ users: OrgUser[]; groups: OrgGroup[] }>();
	let loadingUsersAndGroups = $state(false);
	let models = $state<Model[]>([]);
	let defaultModelAliases = $derived(defaultModelAliasesStore.current);
	let llmModels = $derived(models.filter((model) => model.usage === ModelUsage.LLM));
	let loadingModels = $state(true);

	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();
	let addModelDialog = $state<ReturnType<typeof SearchModels>>();

	let deletingPolicy = $state(false);

	let initialPolicyJson = $derived(
		initialModelAccessPolicy
			? JSON.stringify({
					subjects: initialModelAccessPolicy.subjects,
					models: initialModelAccessPolicy.models
				})
			: ''
	);

	let hasChanges = $derived(
		!initialPolicyJson ||
			JSON.stringify({
				subjects: modelAccessPolicy.subjects,
				models: modelAccessPolicy.models
			}) !== initialPolicyJson
	);

	onMount(async () => {
		const fetchedModels = await AdminService.listModels({ all: true });
		models = fetchedModels;
		loadingModels = false;
	});

	let modelsMap = $derived(new Map(models.map((model) => [model.id, model])));

	// Map alias name -> Model for quick lookups
	let aliasToModelMap = $derived(
		new Map(defaultModelAliases.map((alias) => [alias.alias, modelsMap.get(alias.model)]))
	);

	// Separate model resources into aliases vs regular models
	let { aliasResources, regularModelResources } = $derived.by(() => {
		const aliases: ModelResource[] = [];
		const regular: ModelResource[] = [];

		for (const modelResource of modelAccessPolicy.models ?? []) {
			if (modelResource.id.startsWith('obot://')) {
				aliases.push(modelResource);
			} else {
				regular.push(modelResource);
			}
		}

		return { aliasResources: aliases, regularModelResources: regular };
	});

	// Convert alias resources to display data
	let aliasesTableData = $derived.by(() => {
		return aliasResources.map((resource) => {
			const aliasName = resource.id.replace('obot://', '');
			const model = aliasToModelMap.get(aliasName as ModelAlias);
			const isAllowed = aliasName === ModelAlias.Llm || aliasName === ModelAlias.LlmMini;

			return {
				id: resource.id,
				aliasName: aliasName,
				aliasLabel: ModelAliasLabels[aliasName as ModelAlias] || aliasName,
				usage: model?.usage,
				effectiveModelName: model?.displayName || model?.targetModel || m.models_not_configured(),
				isConfigured: !!model,
				warning: isAllowed ? undefined : llmModelWarning
			};
		});
	});

	let modelsTableData = $derived.by(() => {
		if (modelsMap) {
			return convertModelsToTableData(regularModelResources);
		}
		return [];
	});

	// Combined table data for all models (aliases + regular models)
	let combinedModelsTableData = $derived.by(() => {
		const aliasRows = aliasesTableData.map((alias) => {
			const aliasName = alias.id.replace('obot://', '');
			const effectiveModel = aliasToModelMap.get(aliasName as ModelAlias);

			return {
				id: alias.id,
				aliasName: aliasName,
				name: alias.aliasLabel,
				provider: effectiveModel?.modelProviderName || '-',
				effectiveModel: alias.effectiveModelName,
				isAlias: true,
				isConfigured: alias.isConfigured,
				usage: alias.usage,
				isPattern: false,
				matchCount: 0,
				warning: alias.warning
			};
		});

		const regularRows = modelsTableData.map((model) => ({
			id: model.id,
			aliasName: undefined,
			name: model.name,
			provider: model.provider,
			usage: model.usage,
			effectiveModel: null,
			isAlias: false,
			isConfigured: true,
			isPattern: model.isPattern,
			matchCount: model.matchCount,
			warning: model.warning
		}));

		return [...aliasRows, ...regularRows];
	});

	let invalidModelResourceCount = $derived(
		combinedModelsTableData.filter((model) => model.warning).length
	);

	$effect(() => {
		// Prevent loading users and groups if rule has no subjects
		if (!modelAccessPolicy.subjects || modelAccessPolicy.subjects?.length === 0) {
			loadingUsersAndGroups = false;
			return;
		}

		loadingUsersAndGroups = true;

		// Groups are resolved by ID, not listed: the directory can hold tens of thousands of them and
		// only the ones attached here are needed.
		const controller = new AbortController();

		resolveSubjects(
			modelAccessPolicy.subjects,
			untrack(() => usersAndGroups),
			{ signal: controller.signal }
		)
			.then((resolved) => {
				if (controller.signal.aborted) return;
				usersAndGroups = resolved;
				loadingUsersAndGroups = false;
			})
			.catch((error) => {
				if (controller.signal.aborted) return;
				console.error('Failed to load users and groups:', error);
				loadingUsersAndGroups = false;
			});

		return () => controller.abort();
	});

	function convertModelsToTableData(modelResources: ModelResource[]) {
		return modelResources.map((model) => {
			if (model.id === '*') {
				return {
					id: model.id,
					name: m.models_all_models(),
					usage: undefined,
					provider: '-',
					isPattern: false,
					matchCount: 0
				};
			}

			if (model.id.endsWith('*')) {
				// Wildcard suffix pattern; count the currently matching models
				// (case-sensitive prefix match on target model, like the backend)
				const prefix = model.id.slice(0, -1);
				const matchCount = models.filter((candidate) =>
					(candidate.targetModel || '').startsWith(prefix)
				).length;

				return {
					id: model.id,
					name: model.id,
					usage: undefined,
					provider: '-',
					isPattern: true,
					matchCount
				};
			}

			const found = modelsMap.get(model.id);
			let warning: string | undefined;
			if (!found) {
				warning = m.models_access_policies_model_no_longer_exists();
			} else if (found.usage !== ModelUsage.LLM && found.usage !== ModelAlias.LlmMini) {
				warning = llmModelWarning;
			}
			return {
				id: model.id,
				name: found?.displayName || found?.name || model.id,
				usage: found?.usage,
				provider: found?.modelProviderName || '-',
				isPattern: false,
				matchCount: 0,
				warning
			};
		});
	}

	function validate(policy: typeof modelAccessPolicy) {
		if (!policy) return false;

		if (invalidModelResourceCount > 0) {
			return false;
		}

		return (
			policy.displayName.length > 0 &&
			(policy.subjects?.length ?? 0) > 0 &&
			(policy.models?.length ?? 0) > 0
		);
	}
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if modelAccessPolicy.id}
			<div class="flex w-full items-center justify-between gap-4">
				<div class="flex items-center gap-2">
					<h1 class="flex items-center gap-4 text-2xl font-semibold">
						{modelAccessPolicy.displayName}
					</h1>
				</div>
				{#if !readonly}
					<IconButton
						variant="danger2"
						tooltip={{ text: m.core_delete_policy() }}
						onclick={() => {
							deletingPolicy = true;
						}}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			</div>
		{/if}

		{#if !modelAccessPolicy.id}
			<div
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 rounded-lg border border-transparent p-4"
			>
				<div class="flex flex-col gap-6">
					<div class="flex flex-col gap-2">
						<label for="model-access-policy-name" class="flex-1 text-sm font-light capitalize">
							{m.core_name()}
						</label>
						<input
							id="model-access-policy-name"
							bind:value={modelAccessPolicy.displayName}
							class="text-input-filled mt-0.5"
							disabled={readonly}
						/>
					</div>
				</div>
			</div>
		{/if}

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">{m.core_users_and_groups()}</h2>
				{#if !readonly}
					<div class="relative flex items-center gap-4">
						{#if loadingUsersAndGroups}
							<button class="btn btn-primary flex items-center gap-1 text-sm" disabled>
								<Plus class="size-4" />
								{m.core_add_user_group()}
							</button>
						{:else}
							<button
								class="btn btn-primary flex items-center gap-1 text-sm"
								onclick={() => {
									addUserGroupDialog?.open();
								}}
							>
								<Plus class="size-4" />
								{m.core_add_user_group()}
							</button>
						{/if}
					</div>
				{/if}
			</div>
			{#if loadingUsersAndGroups}
				<div class="my-2 flex items-center justify-center">
					<Loading class="size-6" />
				</div>
			{:else}
				{@const tableData = convertSubjectsToTableData(
					modelAccessPolicy.subjects ?? [],
					usersAndGroups?.users ?? [],
					usersAndGroups?.groups ?? []
				)}
				<Table
					data={tableData}
					fields={['displayName', 'type']}
					headers={[
						{ property: 'displayName', title: m.core_name() },
						{ property: 'type', title: m.core_type() }
					]}
					noDataMessage={m.core_no_users_or_groups_added()}
				>
					{#snippet actions(d)}
						{#if !readonly}
							<IconButton
								variant="danger"
								onclick={() => {
									modelAccessPolicy.subjects = modelAccessPolicy.subjects?.filter(
										(subject) => resolveSubjectPickerById(subject) !== d.id
									);
								}}
								tooltip={{ text: m.core_delete_user_group() }}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					{/snippet}
				</Table>
			{/if}
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">{m.models_title()}</h2>
				{#if !readonly}
					<button
						class="btn btn-primary flex items-center gap-1 text-sm"
						onclick={() => {
							addModelDialog?.open();
						}}
					>
						<Plus class="size-4" />
						{m.models_access_policies_add_model()}
					</button>
				{/if}
			</div>
			{#if loadingModels}
				<div class="my-2 flex items-center justify-center">
					<Loading class="size-6" />
				</div>
			{:else}
				{#if invalidModelResourceCount > 0}
					<div class="notification-alert flex items-start gap-1" role="alert">
						<TriangleAlert class="text-warning size-4 shrink-0" />
						<p class="text-xs">
							{m.models_access_policies_invalid_models_notice()}
						</p>
					</div>
				{/if}
				<Table
					data={combinedModelsTableData}
					fields={['name', 'provider']}
					headers={[
						{ property: 'name', title: m.models_access_policies_col_model() },
						{ property: 'provider', title: m.core_col_provider() }
					]}
					noDataMessage={m.models_access_policies_no_models_added()}
				>
					{#snippet onRenderColumn(field, d)}
						{#if field === 'name'}
							{#if d.isAlias}
								<div class="flex flex-col">
									<div class="flex items-center gap-2">
										<span class="font-medium">{d.aliasName}</span>
										<span class="text-muted-content text-xs" class:text-warning={!d.isConfigured}>
											{d.effectiveModel}
										</span>
									</div>
									<span class="text-muted-content text-xs">{d.name}</span>
								</div>
							{:else if d.isPattern}
								<div class="flex flex-col">
									<div class="flex items-center gap-2">
										<span class="font-mono font-medium">{d.name}</span>
										<span
											class="bg-base-300 dark:bg-base-400 rounded-full px-2 py-0.5 text-xs font-medium"
										>
											{m.models_access_policies_pattern()}
										</span>
									</div>
									<span class="text-muted-content text-xs">
										{#if d.matchCount > 0}
											{d.matchCount === 1
												? m.models_access_policies_pattern_matches_one({ count: d.matchCount })
												: m.models_access_policies_pattern_matches_other({ count: d.matchCount })}
										{:else}
											{m.models_access_policies_pattern_no_matches()}
										{/if}
									</span>
								</div>
							{:else}
								<div class="flex flex-col">
									<span class="font-medium">{d.name}</span>
									{#if d.usage}
										<span class="text-muted-content text-xs">
											{ModelUsageLabels[d.usage as ModelUsage] || d.usage}
										</span>
									{/if}
								</div>
							{/if}
							{#if d.warning}
								<div class="ml-2 text-warning flex items-center gap-1 text-xs">
									<TriangleAlert class="size-3 shrink-0" />
									<span>{d.warning}</span>
								</div>
							{/if}
						{:else}
							{d[field as keyof typeof d]}
						{/if}
					{/snippet}
					{#snippet actions(d)}
						{#if !readonly}
							<IconButton
								variant="danger"
								onclick={() => {
									modelAccessPolicy.models =
										modelAccessPolicy.models?.filter((model) => model.id !== d.id) ?? [];
								}}
								tooltip={{ text: m.models_access_policies_remove_model() }}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					{/snippet}
				</Table>
			{/if}
		</div>
	</div>
	{#if !readonly}
		<div
			class="bg-base-200 text-muted-content dark:bg-base-100 sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4"
			out:fly={{ x: -100, duration }}
			in:fly={{ x: -100 }}
		>
			<div class="flex w-full justify-end gap-2">
				{#if !modelAccessPolicy.id}
					<button
						class="btn btn-secondary text-sm"
						onclick={() => {
							goto('/models?view=access-policies');
						}}
					>
						{m.common_cancel()}
					</button>
					<button
						class="btn btn-primary text-sm"
						disabled={!validate(modelAccessPolicy) || saving}
						onclick={async () => {
							saving = true;
							try {
								const response = await AdminService.createModelAccessPolicy(modelAccessPolicy);
								modelAccessPolicy = response;
								onCreate?.(response);
							} finally {
								saving = false;
							}
						}}
					>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							{m.core_save()}
						{/if}
					</button>
				{:else}
					<button
						class="btn btn-primary text-sm"
						disabled={!validate(modelAccessPolicy) || !hasChanges || saving}
						onclick={async () => {
							if (!modelAccessPolicy.id) return;
							saving = true;
							try {
								const response = await AdminService.updateModelAccessPolicy(
									modelAccessPolicy.id,
									modelAccessPolicy
								);
								modelAccessPolicy = response;
								onUpdate?.(response);
							} finally {
								saving = false;
							}
						}}
					>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							{m.core_update()}
						{/if}
					</button>
				{/if}
			</div>
		</div>
	{/if}
</div>

<SearchUsers
	bind:this={addUserGroupDialog}
	filterIds={modelAccessPolicy.subjects?.map(resolveSubjectPickerById) ?? []}
	onAdd={async (users: OrgUser[], groups: OrgGroup[]) => {
		const existingSubjectIds = new Set(
			modelAccessPolicy.subjects?.map(resolveSubjectPickerById) ?? []
		);
		const newSubjects = [
			...users
				.filter((user: OrgUser) => !existingSubjectIds.has(user.id))
				.map((user: OrgUser) => ({
					type: 'user' as const,
					id: user.id
				})),
			...groups
				.filter((group: OrgGroup) => !existingSubjectIds.has(group.id))
				.map((group: OrgGroup) => resolveSubjectFromGroup(group))
		];
		modelAccessPolicy.subjects = [...(modelAccessPolicy.subjects ?? []), ...newSubjects];
	}}
/>

<SearchModels
	bind:this={addModelDialog}
	models={llmModels}
	defaultAliases={defaultModelAliases}
	exclude={modelAccessPolicy.models?.map((model) => model.id) ?? []}
	onAdd={async (modelIds: string[]) => {
		const existingModelIds = new Set(modelAccessPolicy.models?.map((model) => model.id) ?? []);
		const newModels = modelIds.filter((id) => !existingModelIds.has(id)).map((id) => ({ id: id }));

		modelAccessPolicy.models = [...(modelAccessPolicy.models ?? []), ...newModels];
	}}
/>

<Confirm
	msg={modelAccessPolicy.displayName
		? m.core_delete_named_component({ name: modelAccessPolicy.displayName })
		: m.core_delete_this_policy()}
	show={deletingPolicy}
	onsuccess={async () => {
		if (!modelAccessPolicy.id) return;
		saving = true;
		await AdminService.deleteModelAccessPolicy(modelAccessPolicy.id);
		goto('/models?view=access-policies');
	}}
	oncancel={() => (deletingPolicy = false)}
/>
