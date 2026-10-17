<script lang="ts">
	import Select from '$lib/components/Select.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { m } from '$lib/i18n';
	import { UserService, type Model } from '$lib/services';
	import {
		formatMetric,
		getBudgetUsage,
		METRIC_LABELS,
		windowLabel,
		WINDOW_UNIT_LABELS,
		type Budget,
		type BudgetLimit,
		type BudgetMetric,
		type BudgetUsageEntry
	} from '$lib/services/llm-gateway/gateway';
	import { Plus, Trash2 } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';

	// The fields of one budget of a profile. It edits a copy and hands the result to onSave; the
	// profile form keeps it in its draft until the profile is saved.
	interface Props {
		// The budget to edit, or undefined for a new one.
		budget?: Budget;
		// The models the budget's profile grants, as the profile form has them now. A budget
		// covers only its profile's models, so only they can be chosen.
		models: Model[];
		readonly?: boolean;
		onSave: (budget: Budget) => void;
		onCancel: () => void;
	}

	let { budget: initial, models, readonly = false, onSave, onCancel }: Props = $props();

	const units = ['h', 'd', 'w', 'M'];
	const metricOptions = (Object.entries(METRIC_LABELS) as [BudgetMetric, string][]).map(
		([id, label]) => ({ id, label })
	);
	// Select needs truthy option ids, so the window type is a string rather than a boolean.
	const windowTypeOptions = [
		{ id: 'rolling', label: m.models_budgets_window_rolling() },
		{ id: 'calendar', label: m.models_budgets_window_calendar() }
	];
	const selectClass =
		'bg-base-200 dark:bg-base-200 dark:border-base-400 border border-transparent shadow-inner';

	type LimitRow = {
		metric: BudgetMetric;
		amount: number;
		count: number;
		unit: string;
		calendar: boolean;
	};

	function toRow(l: BudgetLimit): LimitRow {
		const match = /^(\d+)([mhdwM])$/.exec(l.window);
		return {
			metric: l.metric,
			amount: l.amount,
			count: match ? Number(match[1]) : 1,
			unit: match ? match[2] : 'd',
			calendar: !!l.calendar
		};
	}

	function toLimit(r: LimitRow): BudgetLimit {
		return {
			metric: r.metric,
			amount: Number(r.amount),
			window: `${r.calendar ? 1 : Math.max(1, Number(r.count))}${r.unit}`,
			calendar: r.calendar
		};
	}

	const start = untrack(() => initial);
	// A budget that is saved already; a new one starts enabled and has no usage yet.
	const saved = !!start?.id;

	let name = $state(start?.name ?? '');
	let description = $state(start?.description ?? '');
	let enabled = $state(start?.enabled ?? true);
	let perUser = $state(start?.perUser ?? true);
	let exemptAdmins = $state(start?.exemptAdmins ?? false);
	let userTokenLimits = $state(start?.userTokenLimits ?? false);
	let limits = $state<LimitRow[]>(
		start
			? start.limits.map(toRow)
			: [{ metric: 'spend', amount: 50, count: 1, unit: 'M', calendar: true }]
	);
	let selectedModels = $state<string[]>(start?.models ?? []);
	let specificModels = $state(!!start?.models?.length);
	let modelSearch = $state('');
	let usage = $state<BudgetUsageEntry[]>([]);
	let userNames = $state<Record<string, string>>({});
	const savedLimits = start?.id ? start.limits : [];

	onMount(async () => {
		if (!start?.id) return;
		try {
			usage = await getBudgetUsage(start.id, { dontLogErrors: true });
			if (start.perUser && usage.length) {
				const users = await UserService.listUsers().catch(() => []);
				userNames = Object.fromEntries(
					users.map((u) => [u.id, u.displayName || u.email || u.username || u.id])
				);
			}
		} catch {
			// A budget only kept in the form's draft has no usage yet.
			usage = [];
		}
	});

	let grantedIDs = $derived(new Set(models.map((model) => model.id)));
	// Models the profile no longer grants are dropped.
	let chosenModels = $derived(selectedModels.filter((id) => grantedIDs.has(id)));
	let visibleModels = $derived(
		models.filter(
			(model) =>
				!modelSearch ||
				(model.targetModel ?? model.name).toLowerCase().includes(modelSearch.toLowerCase())
		)
	);

	// An empty, negative or non-numeric limit can't be saved, and says so.
	function limitValid(l: LimitRow) {
		return l.amount !== null && `${l.amount}` !== '' && Number(l.amount) >= 0;
	}

	let valid = $derived(
		name.trim().length > 0 &&
			limits.length > 0 &&
			limits.every(limitValid) &&
			(!specificModels || chosenModels.length > 0)
	);

	function save() {
		onSave({
			...(start ?? {}),
			name: name.trim(),
			description,
			enabled,
			perUser,
			exemptAdmins,
			userTokenLimits: perUser && userTokenLimits,
			limits: limits.map(toLimit),
			models: specificModels ? chosenModels : [],
			profileID: start?.profileID ?? ''
		});
	}

	let usageRows = $derived(
		[...usage]
			.sort((a, b) => (b.used[0] ?? 0) - (a.used[0] ?? 0))
			.map((u) => ({
				...u,
				id: u.userID ?? 'shared',
				label: u.userID ? (userNames[u.userID] ?? u.userID) : m.models_budgets_everyone_shared()
			}))
	);
	let usageFields = $derived(['label', ...savedLimits.map((_, i) => `limit-${i}`)]);
	let usageHeaders = $derived([
		{ property: 'label', title: perUser ? m.core_col_user() : m.models_budgets_allowance() },
		...savedLimits.map((l, i) => ({
			property: `limit-${i}`,
			title: m.models_budgets_usage_column({
				metric: METRIC_LABELS[l.metric],
				window: windowLabel(l)
			})
		}))
	]);

	function unitOptions(l: LimitRow) {
		return units.map((id) => ({
			id,
			label: WINDOW_UNIT_LABELS[id][!l.calendar && l.count > 1 ? 'other' : 'one']
		}));
	}
</script>

<div class="flex flex-col gap-4">
	<p class="text-muted-content text-sm">{m.models_budgets_details_description()}</p>
	{#if saved}
		<div>
			<Toggle
				label={m.core_status_enabled()}
				labelInline
				checked={enabled}
				disabled={readonly}
				onChange={(v) => (enabled = v)}
			/>
			<p class="text-muted-content mt-1 text-xs">{m.models_budgets_enabled_hint()}</p>
		</div>
	{/if}
	<div class="flex flex-col gap-1">
		<label for="budget-name" class="text-sm font-light">{m.core_name()}</label>
		<input id="budget-name" class="text-input-filled" bind:value={name} disabled={readonly} />
	</div>
	<div class="flex flex-col gap-1">
		<label for="budget-description" class="text-sm font-light">{m.core_description()}</label>
		<input
			id="budget-description"
			class="text-input-filled"
			bind:value={description}
			disabled={readonly}
		/>
	</div>
	<div class="flex flex-col gap-2">
		<span class="text-sm font-light">{m.models_budgets_allowance()}</span>
		<label class="flex items-center gap-2 text-sm">
			<input
				type="radio"
				class="radio radio-sm"
				name="scope"
				checked={perUser}
				onchange={() => (perUser = true)}
				disabled={readonly}
			/>
			{m.models_budgets_scope_per_user()}
		</label>
		<label class="flex items-center gap-2 text-sm">
			<input
				type="radio"
				class="radio radio-sm"
				name="scope"
				checked={!perUser}
				onchange={() => (perUser = false)}
				disabled={readonly}
			/>
			{m.models_budgets_scope_shared()}
		</label>
		<span class="text-muted-content text-xs">{m.models_budgets_stacking_note()}</span>
	</div>
	<Toggle
		label={m.models_budgets_exempt_admins()}
		labelInline
		checked={exemptAdmins}
		disabled={readonly}
		onChange={(v) => (exemptAdmins = v)}
	/>
	{#if perUser}
		<Toggle
			label={m.models_budgets_user_token_limits()}
			labelInline
			checked={userTokenLimits}
			disabled={readonly}
			onChange={(v) => (userTokenLimits = v)}
		/>
	{/if}

	<section class="flex flex-col gap-3">
		<div class="divider my-1 text-sm font-semibold">{m.models_budgets_limits()}</div>
		<div class="flex flex-wrap items-start justify-between gap-2">
			<p class="text-muted-content max-w-xl text-xs">{m.models_budgets_limits_description()}</p>
			{#if !readonly}
				<button
					type="button"
					class="btn btn-secondary btn-sm flex items-center gap-1 text-xs"
					onclick={() =>
						limits.push({
							metric: 'tokens',
							amount: 1000000,
							count: 1,
							unit: 'd',
							calendar: false
						})}
				>
					<Plus class="size-4" />
					{m.models_budgets_add_limit()}
				</button>
			{/if}
		</div>
		{#each limits as l, i (i)}
			<div class="flex flex-wrap items-end gap-3">
				<div class="flex flex-col gap-1">
					<span id={`limit-metric-${i}-label`} class="text-sm font-light"
						>{m.models_budgets_metric()}</span
					>
					<Select
						id={`limit-metric-${i}`}
						class={selectClass}
						classes={{ root: 'w-44' }}
						ariaLabelledby={`limit-metric-${i}-label`}
						options={metricOptions}
						selected={l.metric}
						onSelect={(option) => (l.metric = option.id)}
						disabled={readonly}
					/>
				</div>
				<div class="flex flex-col gap-1">
					<label for={`limit-amount-${i}`} class="text-sm font-light"
						>{m.models_budgets_limit()}</label
					>
					<input
						id={`limit-amount-${i}`}
						type="number"
						min="0"
						step={l.metric === 'spend' ? '0.01' : '1'}
						class="text-input-filled w-32"
						bind:value={l.amount}
						disabled={readonly}
						aria-invalid={!limitValid(l)}
						aria-describedby={limitValid(l) ? undefined : `limit-amount-${i}-error`}
					/>
				</div>
				<div class="flex flex-col gap-1">
					<span class="text-sm font-light">{m.models_budgets_window()}</span>
					<div class="flex items-center gap-2">
						<Select
							id={`limit-window-type-${i}`}
							class={selectClass}
							classes={{ root: 'w-32' }}
							placeholder={m.models_budgets_window_type()}
							options={windowTypeOptions}
							selected={l.calendar ? 'calendar' : 'rolling'}
							onSelect={(option) => (l.calendar = option.id === 'calendar')}
							disabled={readonly}
						/>
						{#if !l.calendar}
							<input
								type="number"
								min="1"
								class="text-input-filled w-20"
								aria-label={m.models_budgets_window_length()}
								bind:value={l.count}
								disabled={readonly}
							/>
						{/if}
						<Select
							id={`limit-window-unit-${i}`}
							class={selectClass}
							classes={{ root: 'w-32' }}
							placeholder={m.models_budgets_window_unit()}
							options={unitOptions(l)}
							selected={l.unit}
							onSelect={(option) => (l.unit = option.id)}
							disabled={readonly}
						/>
					</div>
				</div>
				{#if !readonly}
					<IconButton
						variant="danger"
						tooltip={{ text: m.models_budgets_remove_limit() }}
						onclick={() => limits.splice(i, 1)}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			</div>
			{#if !limitValid(l)}
				<span id={`limit-amount-${i}-error`} role="alert" class="text-error text-xs">
					{m.models_budgets_limit_invalid()}
				</span>
			{/if}
		{:else}
			<span class="text-muted-content text-sm">{m.models_budgets_add_at_least_one_limit()}</span>
		{/each}
	</section>

	<section class="flex flex-col gap-3">
		<div class="divider my-1 text-sm font-semibold">{m.models_title()}</div>
		<p class="text-muted-content text-xs">{m.models_budgets_models_description()}</p>
		<fieldset class="flex flex-col gap-2">
			<label class="flex cursor-pointer items-start gap-3 rounded-lg border border-base-300 p-3">
				<input
					type="radio"
					class="radio radio-primary radio-sm mt-0.5"
					name="budget-model-scope"
					checked={!specificModels}
					disabled={readonly}
					onchange={() => (specificModels = false)}
				/>
				<span>
					<span class="block text-sm font-medium">{m.models_budgets_all_models()}</span>
					<span class="block text-xs font-light text-muted-content"
						>{m.models_budgets_all_models_description()}</span
					>
				</span>
			</label>
			<label class="flex cursor-pointer items-start gap-3 rounded-lg border border-base-300 p-3">
				<input
					type="radio"
					class="radio radio-primary radio-sm mt-0.5"
					name="budget-model-scope"
					checked={specificModels}
					disabled={readonly}
					onchange={() => (specificModels = true)}
				/>
				<span>
					<span class="block text-sm font-medium">{m.models_budgets_specific_models()}</span>
					<span class="block text-xs font-light text-muted-content"
						>{m.models_budgets_specific_models_description()}</span
					>
				</span>
			</label>
		</fieldset>
		{#if specificModels}
			<div class="flex flex-col gap-2">
				<input
					class="text-input-filled"
					placeholder={m.models_budgets_search_models()}
					bind:value={modelSearch}
				/>
				<div class="grid max-h-64 grid-cols-1 gap-1 overflow-y-auto md:grid-cols-2">
					{#each visibleModels as model (model.id)}
						<label class="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								class="checkbox checkbox-sm"
								checked={chosenModels.includes(model.id)}
								disabled={readonly}
								onchange={(e) => {
									const checked = (e.target as HTMLInputElement).checked;
									selectedModels = checked
										? [...chosenModels, model.id]
										: chosenModels.filter((x) => x !== model.id);
								}}
							/>
							<span class="font-mono text-xs">{model.targetModel ?? model.name}</span>
						</label>
					{:else}
						<span class="text-muted-content text-sm">{m.models_budgets_no_profile_models()}</span>
					{/each}
				</div>
				<span class="text-muted-content text-xs">
					{chosenModels.length
						? m.models_budgets_models_selected({ count: chosenModels.length })
						: m.models_budgets_select_a_model()}
				</span>
			</div>
		{/if}
	</section>

	{#if savedLimits.length}
		<section class="flex flex-col gap-3">
			<div class="divider my-1 text-sm font-semibold">{m.models_budgets_current_usage()}</div>
			<p class="text-muted-content text-xs">{m.models_budgets_current_usage_description()}</p>
			<Table
				data={usageRows}
				fields={usageFields}
				headers={usageHeaders}
				noDataMessage={m.models_budgets_no_usage()}
			>
				{#snippet onRenderColumn(property, row)}
					{#if property === 'label'}
						{row.label}
					{:else}
						{@const i = Number(property.slice('limit-'.length))}
						{@const l = savedLimits[i]}
						{@const used = row.used[i] ?? 0}
						<div class="flex items-center gap-2">
							<progress
								class={`progress w-24 ${used >= l.amount ? 'progress-error' : 'progress-primary'}`}
								value={Math.min(used, l.amount)}
								max={l.amount || 1}
							></progress>
							<span class="text-xs">
								{formatMetric(l.metric, used)} / {formatMetric(l.metric, l.amount)}
							</span>
						</div>
					{/if}
				{/snippet}
			</Table>
		</section>
	{/if}

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
