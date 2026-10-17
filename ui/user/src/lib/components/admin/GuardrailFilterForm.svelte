<script lang="ts">
	import { resolve } from '$app/paths';
	import { MCP_FILTERS_FIELD_IDS, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type MCPFilter,
		type MCPFilterInput,
		type MCPFilterManifest,
		type MCPFilterResource,
		type MCPFilterWebhookSelector,
		type MCPGuardrail,
		type MCPGuardrailProvider,
		type MCPGuardrailFilterAction,
		type MCPGuardrailFilterDirection,
		type OrgGroup,
		type OrgUser
	} from '$lib/services';
	import { guardrailTypeLabel } from '$lib/services/llm-gateway/guardrail-text';
	import { convertSubjectsToTableData, resolveSubjects } from '$lib/subjectResolver';
	import Select from '../Select.svelte';
	import Toggle from '../Toggle.svelte';
	import NewGuardrailDialog from '../guardrails/NewGuardrailDialog.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import Table from '../table/Table.svelte';
	import SearchUsers from './SearchUsers.svelte';
	import SelectorsAndResourcesFormSegment from './SelectorsAndResourcesFormSegment.svelte';
	import { Plus, Trash2, TriangleAlert } from '@lucide/svelte';
	import { onMount, untrack, type Snippet } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		topContent?: Snippet;
		filter?: MCPFilterInput;
		onCreate?: (filter?: MCPFilter) => void;
		onUpdate?: (filter?: MCPFilter) => void;
		readonly?: boolean;
	}

	let { topContent, filter: initialFilter, onCreate, onUpdate, readonly }: Props = $props();

	const duration = PAGE_TRANSITION_DURATION;
	const CONDITION_EXAMPLE = 'mcp_server == "github" && mcp_tool.startsWith("create_")';

	const actionOptions: { value: MCPGuardrailFilterAction; label: string; description: string }[] = [
		{
			value: 'block',
			label: m.guardrails_filter_action_block(),
			description: m.guardrails_filter_action_block_description()
		},
		{
			value: 'redact',
			label: m.guardrails_filter_action_redact(),
			description: m.guardrails_filter_action_redact_description()
		}
	];
	const selectClass =
		'bg-base-200 dark:bg-base-200 dark:border-base-400 border border-transparent shadow-inner';
	const directionOptions: { value: MCPGuardrailFilterDirection; label: string }[] = [
		{ value: 'request', label: m.guardrails_filter_direction_request() },
		{ value: 'response', label: m.guardrails_filter_direction_response() },
		{ value: 'both', label: m.guardrails_filter_direction_both() }
	];
	const conditionVariables = [
		{ name: 'mcp_server', description: m.guardrails_filter_var_mcp_server() },
		{ name: 'mcp_tool', description: m.guardrails_filter_var_mcp_tool() },
		{ name: 'mcp_arguments', description: m.guardrails_filter_var_mcp_arguments() },
		{ name: 'headers', description: m.guardrails_filter_var_headers() },
		{ name: 'user', description: m.guardrails_filter_var_user() }
	];

	let initialFilterId = $derived(initialFilter?.id);

	let filter = $state<{
		name: string;
		resources: MCPFilterResource[];
		selectors: MCPFilterWebhookSelector[];
		disabled: boolean;
		guardrailIDs: string[];
		action: MCPGuardrailFilterAction;
		direction: MCPGuardrailFilterDirection;
		failClosed: boolean;
		samplingRate: number;
		condition: string;
		blockMessage: string;
	}>(
		untrack(() => {
			const guardrail = initialFilter?.guardrail;
			return {
				name: initialFilter?.name ?? '',
				resources: initialFilter?.resources ?? [{ id: 'default', type: 'mcpCatalog' }],
				selectors: initialFilter?.selectors ?? [],
				disabled: initialFilter?.disabled ?? false,
				guardrailIDs: guardrail?.guardrailIDs ?? [],
				// Filters block or redact. Anything else, such as a log-only action saved before it was
				// removed, blocks, as it does when the filter runs.
				action: guardrail?.action === 'redact' ? 'redact' : 'block',
				direction: guardrail?.direction ?? 'both',
				failClosed: guardrail?.failClosed ?? false,
				samplingRate: guardrail?.samplingRate || 100,
				condition: guardrail?.condition ?? '',
				blockMessage: guardrail?.blockMessage ?? ''
			};
		})
	);

	// The "everyone" subject is shown as the "All Obot Users" selector, as in access policies.
	type Subject = { type: 'user' | 'group' | 'selector'; id: string };
	let subjects = $state<Subject[]>(
		untrack(() =>
			(initialFilter?.guardrail?.subjects ?? []).map((s) =>
				s.type === 'everyone'
					? { type: 'selector' as const, id: '*' }
					: { type: s.type, id: s.id ?? '' }
			)
		)
	);
	let resolved = $state<{ users: OrgUser[]; groups: OrgGroup[] }>({ users: [], groups: [] });
	let searchUsersDialog = $state<ReturnType<typeof SearchUsers>>();

	let guardrails = $state<MCPGuardrail[]>([]);
	let providers = $state<MCPGuardrailProvider[]>([]);
	let guardrailsLoading = $state(true);
	let guardrailsLoadError = $state('');
	let newGuardrailDialog = $state<ReturnType<typeof NewGuardrailDialog>>();

	let saving = $state(false);
	let saveError = $state('');
	let showValidation = $state(false);
	let conditionError = $state('');
	let validatingCondition = $state(false);

	onMount(async () => {
		try {
			[guardrails, providers] = await Promise.all([
				AdminService.listMCPGuardrails(),
				AdminService.listMCPGuardrailProviders().catch(() => [] as MCPGuardrailProvider[])
			]);
		} catch (err) {
			guardrailsLoadError = parseErrorContent(err).message;
		} finally {
			guardrailsLoading = false;
		}

		if (subjects.length > 0) {
			resolved = await resolveSubjects(subjects).catch(() => resolved);
		}
	});

	// Validate the condition as it's typed, ignoring answers superseded by later edits.
	let validationSeq = 0;
	$effect(() => {
		const expr = filter.condition.trim();
		const seq = ++validationSeq;
		if (!expr) {
			conditionError = '';
			validatingCondition = false;
			return;
		}
		validatingCondition = true;
		const timer = setTimeout(async () => {
			try {
				const result = await AdminService.validateMCPGuardrailCondition(expr, {
					dontLogErrors: true
				});
				if (seq === validationSeq) conditionError = result.valid ? '' : (result.error ?? '');
			} catch (err) {
				if (seq === validationSeq) conditionError = parseErrorContent(err).message;
			} finally {
				if (seq === validationSeq) validatingCondition = false;
			}
		}, 400);
		return () => clearTimeout(timer);
	});

	let providersByType = $derived(new Map(providers.map((p) => [p.type, p])));
	let guardrailsById = $derived(new Map(guardrails.map((d) => [d.id, d])));
	// Selected guardrails that no longer exist (deleted since the filter was saved).
	let missingGuardrailIDs = $derived(filter.guardrailIDs.filter((id) => !guardrailsById.has(id)));
	let nonRedacting = $derived(
		filter.action === 'redact'
			? guardrails.filter(
					(d) =>
						filter.guardrailIDs.includes(d.id) && providersByType.get(d.type)?.redacts === false
				)
			: []
	);
	let subjectRows = $derived(convertSubjectsToTableData(subjects, resolved.users, resolved.groups));

	let nameError = $derived(showValidation && !filter.name.trim());
	let guardrailsError = $derived(showValidation && filter.guardrailIDs.length === 0);
	let samplingRateError = $derived(
		showValidation &&
			(!Number.isFinite(Number(filter.samplingRate)) ||
				Number(filter.samplingRate) < 1 ||
				Number(filter.samplingRate) > 100)
	);

	function toggleGuardrail(id: string, checked: boolean) {
		filter.guardrailIDs = checked
			? [...filter.guardrailIDs, id]
			: filter.guardrailIDs.filter((d) => d !== id);
	}

	function buildManifest(): MCPFilterManifest {
		const selectors =
			filter.selectors.length > 0
				? filter.selectors
						.map((s) => ({
							...s,
							identifiers: s.identifiers?.filter((id) => id.trim()) || []
						}))
						.filter((s) => s.method || (s.identifiers && s.identifiers.length > 0))
				: undefined;
		const condition = filter.condition.trim();
		const blockMessage = filter.blockMessage.trim();

		return {
			name: filter.name.trim(),
			resources: filter.resources,
			selectors,
			disabled: filter.disabled,
			guardrail: {
				guardrailIDs: filter.guardrailIDs,
				action: filter.action,
				direction: filter.direction,
				failClosed: filter.failClosed,
				samplingRate: Number(filter.samplingRate),
				subjects: subjects.map((s) =>
					s.type === 'selector' ? { type: 'everyone' as const } : { type: s.type, id: s.id }
				),
				...(condition ? { condition } : {}),
				...(blockMessage ? { blockMessage } : {})
			}
		};
	}

	async function handleSave() {
		const samplingRate = Number(filter.samplingRate);
		if (
			!filter.name.trim() ||
			filter.guardrailIDs.length === 0 ||
			!Number.isFinite(samplingRate) ||
			samplingRate < 1 ||
			samplingRate > 100
		) {
			showValidation = true;
			return;
		}
		if (conditionError || validatingCondition) {
			return;
		}

		saving = true;
		saveError = '';
		try {
			const manifest = buildManifest();
			if (initialFilterId) {
				const result = await AdminService.updateMCPFilter(initialFilterId, manifest);
				onUpdate?.(result);
			} else {
				const result = await AdminService.createMCPFilter(manifest);
				onCreate?.(result);
			}
		} catch (err) {
			saveError = parseErrorContent(err).message;
		} finally {
			saving = false;
		}
	}
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if topContent}
			{@render topContent()}
		{/if}
		{#if !initialFilterId}
			<h1 class="text-2xl font-semibold">{m.guardrails_filter_create_title()}</h1>
		{/if}

		<div
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 rounded-lg border border-transparent p-4"
			id={MCP_FILTERS_FIELD_IDS.basicDetails}
		>
			<div class="flex flex-col gap-6">
				<div class="flex flex-col gap-2">
					<label for="filter-name" class="flex-1 text-sm font-light capitalize"
						>{m.core_name()}</label
					>
					<div class="flex grow flex-col gap-0.5">
						<input
							id="filter-name"
							bind:value={filter.name}
							class="text-input-filled dark:bg-base-100 mt-0.5 {nameError
								? 'border-error focus:border-error focus:ring-error'
								: ''}"
							disabled={readonly}
						/>
						{#if nameError}
							<p class="text-xs text-error">{m.mcps_name_required()}</p>
						{/if}
					</div>
				</div>
				<div class="flex flex-col gap-1">
					<span class="text-sm font-light">{m.core_type()}</span>
					<p class="text-sm">{m.guardrails_filter_type_guardrail()}</p>
					<p class="text-muted-content text-xs">
						{m.guardrails_filter_type_description()}
					</p>
				</div>
			</div>
		</div>

		<div
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-6 rounded-lg border border-transparent p-4 shadow-sm"
			id={MCP_FILTERS_FIELD_IDS.runtimeFormDetails}
		>
			<div class="flex flex-col gap-2">
				<div class="flex items-start justify-between gap-4">
					<div class="flex flex-col gap-1">
						<span id="guardrail-list-label" class="text-sm font-light"
							>{m.guardrails_filter_guardrails()}</span
						>
						<p class="text-muted-content text-xs">
							{m.guardrails_filter_guardrails_description()}
						</p>
					</div>
					<div class="flex shrink-0 items-center gap-3">
						<a href={resolve('/models?view=guardrails')} class="text-link text-xs"
							>{m.guardrails_manage()}</a
						>
						{#if !readonly}
							<button
								type="button"
								class="btn btn-secondary flex items-center gap-1 text-xs"
								onclick={() => newGuardrailDialog?.open()}
							>
								<Plus class="size-3" />
								{m.guardrails_new()}
							</button>
						{/if}
					</div>
				</div>
				<div
					role="group"
					aria-labelledby="guardrail-list-label"
					class={twMerge(
						'bg-base-200 dark:bg-base-100 rounded-lg p-3 shadow-inner',
						guardrailsError && 'border-error border'
					)}
				>
					{#if guardrailsLoading}
						<Loading class="size-4" />
					{:else if guardrailsLoadError}
						<p class="text-xs text-error">
							{m.guardrails_filter_guardrails_load_failed({ error: guardrailsLoadError })}
						</p>
					{:else}
						<div class="grid grid-cols-1 gap-2 md:grid-cols-2">
							{#each guardrails as guardrail (guardrail.id)}
								<label class="flex items-start gap-2 text-sm">
									<input
										type="checkbox"
										class="checkbox checkbox-sm mt-0.5"
										checked={filter.guardrailIDs.includes(guardrail.id)}
										disabled={readonly}
										onchange={(e) =>
											toggleGuardrail(guardrail.id, (e.target as HTMLInputElement).checked)}
									/>
									<span class="flex flex-col">
										<span>
											{guardrail.name}
											<span class="text-muted-content text-xs">
												{guardrailTypeLabel(
													providersByType.get(guardrail.type) ?? {
														type: guardrail.type,
														label: guardrail.type
													}
												)}
											</span>
										</span>
										{#if guardrail.error}
											<span class="text-xs text-error">{guardrail.error}</span>
										{/if}
									</span>
								</label>
							{:else}
								<p class="text-muted-content text-sm">
									{m.guardrails_filter_no_guardrails()}
								</p>
							{/each}
						</div>
					{/if}
					{#if missingGuardrailIDs.length > 0 && !guardrailsLoading && !guardrailsLoadError}
						<p class="mt-2 text-xs text-error">
							{missingGuardrailIDs.length === 1
								? m.guardrails_filter_missing_guardrails_one({
										ids: missingGuardrailIDs.join(', ')
									})
								: m.guardrails_filter_missing_guardrails_other({
										ids: missingGuardrailIDs.join(', ')
									})}
						</p>
					{/if}
				</div>
				{#if guardrailsError}
					<p class="text-xs text-error">{m.guardrails_filter_guardrails_required()}</p>
				{/if}
			</div>

			<div class="grid grid-cols-1 gap-6 md:grid-cols-2">
				<div class="flex flex-col gap-2">
					<span id="guardrail-action-label" class="text-sm font-light"
						>{m.guardrails_filter_action()}</span
					>
					<Select
						id="guardrail-action"
						class={selectClass}
						ariaLabelledby="guardrail-action-label"
						options={actionOptions.map((o) => ({ id: o.value, label: o.label }))}
						selected={filter.action}
						onSelect={(option) => (filter.action = option.id as MCPGuardrailFilterAction)}
						disabled={readonly}
					/>
					<p class="text-muted-content text-xs">
						{actionOptions.find((o) => o.value === filter.action)?.description}
					</p>
				</div>
				<div class="flex flex-col gap-2">
					<span id="guardrail-direction-label" class="text-sm font-light"
						>{m.guardrails_filter_direction()}</span
					>
					<Select
						id="guardrail-direction"
						class={selectClass}
						ariaLabelledby="guardrail-direction-label"
						options={directionOptions.map((o) => ({ id: o.value, label: o.label }))}
						selected={filter.direction}
						onSelect={(option) => (filter.direction = option.id as MCPGuardrailFilterDirection)}
						disabled={readonly}
					/>
					<p class="text-muted-content text-xs">{m.guardrails_filter_direction_description()}</p>
				</div>
				<div class="flex flex-col gap-2">
					<label for="guardrail-sampling-rate" class="text-sm font-light"
						>{m.guardrails_filter_sampling_rate()}</label
					>
					<input
						id="guardrail-sampling-rate"
						type="number"
						min="1"
						max="100"
						bind:value={filter.samplingRate}
						class="text-input-filled dark:bg-base-100 {samplingRateError
							? 'border-error focus:border-error focus:ring-error'
							: ''}"
						disabled={readonly}
					/>
					{#if samplingRateError}
						<p class="text-xs text-error">{m.guardrails_filter_sampling_rate_invalid()}</p>
					{:else}
						<p class="text-muted-content text-xs">
							{m.guardrails_filter_sampling_rate_description()}
						</p>
					{/if}
				</div>
				<div class="flex flex-col gap-1 md:pt-7">
					<Toggle
						classes={{
							label: 'text-sm gap-2 font-light text-base-content'
						}}
						label={m.guardrails_filter_fail_closed()}
						labelInline
						checked={filter.failClosed}
						disabled={readonly}
						onChange={(checked) => (filter.failClosed = checked)}
					/>
					<p class="text-muted-content text-xs font-light">
						{m.guardrails_filter_fail_closed_description()}
					</p>
				</div>
			</div>

			{#if nonRedacting.length > 0}
				<div class="notification-alert flex items-start gap-2 rounded-md p-3 text-sm">
					<TriangleAlert class="size-4 shrink-0" />
					<span>
						{nonRedacting.length === 1
							? m.guardrails_filter_non_redacting_one({
									names: nonRedacting.map((d) => d.name).join(', ')
								})
							: m.guardrails_filter_non_redacting_other({
									names: nonRedacting.map((d) => d.name).join(', ')
								})}
					</span>
				</div>
			{/if}

			<div class="flex flex-col gap-2">
				<label for="guardrail-block-message" class="text-sm font-light">
					{m.guardrails_filter_block_message()}
				</label>
				<input
					id="guardrail-block-message"
					bind:value={filter.blockMessage}
					class="text-input-filled dark:bg-base-100"
					placeholder={m.guardrails_filter_block_message_placeholder()}
					disabled={readonly}
				/>
			</div>
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex md:flex-row flex-col md:items-center items-start gap-4 justify-between">
				<div class="flex flex-col gap-1">
					<h2 class="text-lg font-semibold">{m.core_users_and_groups()}</h2>
					<p class="text-muted-content text-sm">
						{m.guardrails_filter_subjects_description()}
					</p>
				</div>
				{#if !readonly}
					<button
						class="btn btn-primary flex items-center gap-1 text-sm"
						onclick={() => searchUsersDialog?.open()}
					>
						<Plus class="size-4" />
						{m.core_add_user_group()}
					</button>
				{/if}
			</div>
			<Table
				data={subjectRows}
				fields={['displayName', 'type']}
				headers={[{ property: 'displayName', title: m.core_name() }]}
				noDataMessage={m.guardrails_filter_subjects_empty()}
			>
				{#snippet actions(row)}
					{#if !readonly}
						<IconButton
							variant="danger"
							tooltip={{ text: m.guardrails_filter_remove_subject() }}
							onclick={() => (subjects = subjects.filter((s) => s.id !== row.id))}
						>
							<Trash2 class="size-4" />
						</IconButton>
					{/if}
				{/snippet}
			</Table>
		</div>

		<div class="flex flex-col gap-2">
			<div class="flex flex-col gap-1">
				<h2 class="text-lg font-semibold">{m.guardrails_filter_condition_optional()}</h2>
				<p class="text-muted-content text-sm">
					{m.guardrails_filter_condition_description_before()}
					<a href="https://cel.dev" target="_blank" rel="noopener noreferrer" class="text-link"
						>CEL</a
					>
					{m.guardrails_filter_condition_description_after()}
				</p>
			</div>
			<div
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-2 rounded-lg border border-transparent p-4"
			>
				<textarea
					id="guardrail-condition"
					aria-label={m.guardrails_filter_condition()}
					class="text-input-filled dark:bg-base-100 min-h-20 font-mono text-sm {conditionError
						? 'border-error focus:border-error focus:ring-error'
						: ''}"
					placeholder={CONDITION_EXAMPLE}
					bind:value={filter.condition}
					disabled={readonly}></textarea>
				{#if conditionError}
					<p class="text-xs text-error" role="alert">{conditionError}</p>
				{/if}
				<div class="text-muted-content grid grid-cols-1 gap-x-4 gap-y-1 text-xs md:grid-cols-2">
					{#each conditionVariables as v (v.name)}
						<span><code class="text-base-content">{v.name}</code> — {v.description}</span>
					{/each}
				</div>
			</div>
		</div>

		<div class="h-px bg-base-400 w-full my-4"></div>

		<SelectorsAndResourcesFormSegment bind:form={filter} {readonly} />

		{#if saveError}
			<div class="notification-error rounded-md p-3 text-sm">{saveError}</div>
		{/if}
	</div>
	{#if !readonly}
		<div
			class="bg-base-200 dark:bg-base-100 dark:text-muted-content sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4 text-gray-400 z-50"
			out:fly={{ x: -100, duration }}
			in:fly={{ x: -100 }}
		>
			<div class="flex w-full justify-between gap-2">
				<div>
					{#if initialFilterId}
						<button
							class={twMerge('text-sm btn btn-soft', filter.disabled ? 'btn-primary' : 'btn-error')}
							disabled={saving}
							onclick={() => {
								filter.disabled = !filter.disabled;
								handleSave();
							}}
						>
							{#if saving}
								<Loading class="size-4" />
							{:else}
								{filter.disabled ? m.mcps_filters_enable() : m.mcps_filters_disable()}
							{/if}
						</button>
					{/if}
				</div>
				<div class="flex gap-2">
					<button
						class="btn btn-secondary"
						onclick={() => {
							if (initialFilterId) {
								onUpdate?.(undefined);
							} else {
								onCreate?.(undefined);
							}
						}}
					>
						{m.common_cancel()}
					</button>
					<button
						id={MCP_FILTERS_FIELD_IDS.saveBtn}
						class="btn btn-primary text-sm"
						disabled={saving || validatingCondition || !!conditionError}
						onclick={handleSave}
					>
						{#if saving}
							<Loading class="size-4" />
						{:else}
							{m.core_save()}
						{/if}
					</button>
				</div>
			</div>
		</div>
	{/if}
</div>

<SearchUsers
	bind:this={searchUsersDialog}
	offerObotAdmin={false}
	filterIds={subjects.map((s) => s.id)}
	onAdd={(users: OrgUser[], groups: OrgGroup[]) => {
		const have = new Set(subjects.map((s) => s.id));
		subjects = [
			...subjects,
			...users.filter((u) => !have.has(u.id)).map((u) => ({ type: 'user' as const, id: u.id })),
			...groups
				.filter((g) => !have.has(g.id))
				.map((g) => ({ type: g.id === '*' ? ('selector' as const) : ('group' as const), id: g.id }))
		];
		resolved = { users: [...resolved.users, ...users], groups: [...resolved.groups, ...groups] };
	}}
/>

<NewGuardrailDialog
	bind:this={newGuardrailDialog}
	onCreated={(g) => {
		if (!g.id) return;
		guardrails = [...guardrails, { id: g.id, name: g.name, type: g.type, error: g.error }];
		filter.guardrailIDs = [...filter.guardrailIDs, g.id];
	}}
/>
