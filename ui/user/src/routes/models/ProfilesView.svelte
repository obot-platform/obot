<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Search from '$lib/components/Search.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { m } from '$lib/i18n';
	import { AdminService } from '$lib/services';
	import type { ModelAccessPolicy } from '$lib/services/admin/types';
	import { listBudgets, type Budget } from '$lib/services/llm-gateway/gateway';
	import {
		listGuardrailPolicies,
		type GuardrailPolicy
	} from '$lib/services/llm-gateway/guardrails';
	import { accessibleModels, profile } from '$lib/stores';
	import {
		convertSubjectsToTableData,
		resolveSubjects,
		type ResolvedSubjects
	} from '$lib/subjectResolver';
	import {
		getTableUrlParamsSort,
		goto,
		resolve,
		setSortUrlParams,
		setUrlParamAndUpdateUrl
	} from '$lib/url';
	import { openUrl } from '$lib/utils';
	import ProfileForm from './profiles/ProfileForm.svelte';
	import { Info, Plus, Trash2, UsersRound } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';

	let profiles = $state<ModelAccessPolicy[]>([]);
	let budgets = $state<Budget[]>([]);
	let policies = $state<GuardrailPolicy[]>([]);
	let loading = $state(true);
	let usersAndGroups = $state<ResolvedSubjects>();
	let toDelete = $state<ModelAccessPolicy>();

	let query = $derived(page.url.searchParams.get('query') || '');
	let initSort = $derived(getTableUrlParamsSort());
	let readonly = $derived(profile.current.isAdminReadonly?.());
	let showCreate = $derived(page.url.searchParams.has('new') && !readonly);
	const duration = PAGE_TRANSITION_DURATION;

	// How many users and groups a row names before it says how many more there are; the cell's
	// tooltip names them all.
	const SUBJECTS_SHOWN = 3;

	let rows = $derived(
		profiles.map((p) => {
			const names = subjectNames(p);
			return {
				...p,
				name: p.displayName || p.id,
				subjectsLabel: subjectsLabel(p, names),
				subjectsTitle: names.join('\n'),
				allModels: p.models?.some((model) => model.id === '*') ?? false,
				modelCount: p.models?.length ?? 0,
				budgetCount: count(budgets, p.id),
				guardrailCount: count(
					policies.filter((r) => r.target === 'llm'),
					p.id
				)
			};
		})
	);

	let filteredRows = $derived(
		rows.filter((r) => r.name.toLowerCase().includes(query.toLowerCase()))
	);

	async function load() {
		loading = true;
		try {
			[profiles, budgets, policies] = await Promise.all([
				AdminService.listModelAccessPolicies(),
				listBudgets().catch(() => []),
				listGuardrailPolicies().catch(() => [])
			]);
		} finally {
			loading = false;
		}
		resolveSubjects(profiles.flatMap((p) => p.subjects ?? []))
			.then((resolved) => (usersAndGroups = resolved))
			.catch((err) => console.error('Failed to load users and groups:', err));
	}
	onMount(load);

	function count<T extends { profileID: string }>(items: T[], id: string) {
		return items.filter((i) => i.profileID === id).length;
	}

	function subjectNames(p: ModelAccessPolicy) {
		if (!usersAndGroups) return [];
		return convertSubjectsToTableData(
			p.subjects ?? [],
			usersAndGroups.users,
			usersAndGroups.groups
		).map((row) => row.displayName);
	}

	function updateQuery(value: string) {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	}

	function subjectsLabel(p: ModelAccessPolicy, names: string[]) {
		const subjects = p.subjects ?? [];
		if (subjects.some((s) => s.type === 'selector' && s.id === '*'))
			return m.models_profiles_everyone();
		if (names.length > 0) {
			const shown = names.slice(0, SUBJECTS_SHOWN).join(', ');
			return names.length > SUBJECTS_SHOWN
				? m.models_profiles_subjects_more({
						names: shown,
						count: names.length - SUBJECTS_SHOWN
					})
				: shown;
		}
		const users = subjects.filter((s) => s.type === 'user').length;
		const groups = subjects.length - users;
		return (
			[
				users &&
					(users === 1
						? m.models_profiles_users_one({ count: users })
						: m.models_profiles_users_other({ count: users })),
				groups &&
					(groups === 1
						? m.models_profiles_groups_one({ count: groups })
						: m.models_profiles_groups_other({ count: groups }))
			]
				.filter(Boolean)
				.join(', ') || m.models_profiles_no_one()
		);
	}
</script>

{#if showCreate}
	<div in:fly|global={{ x: 100, duration }} out:fly|global={{ x: -100, duration }}>
		<ProfileForm />
	</div>
{:else}
	<div>
		{#if loading}
			<Skeleton type="table" count={5} classes={{ header: 'h-14' }} />
		{:else if profiles.length === 0}
			<div class="mt-12 flex flex-col items-center justify-center text-center">
				<div
					class="bg-primary/10 text-primary mb-4 flex size-9 items-center justify-center rounded-md"
				>
					<UsersRound class="size-4" />
				</div>
				<h2 class="font-semibold">{m.models_profiles_empty_title()}</h2>
				<p class="text-muted-content mt-1 max-w-sm text-xs">
					{m.models_profiles_empty_description()}
				</p>
				{#if !readonly}
					<button
						class="btn btn-primary btn-sm mt-5"
						onclick={() => goto('/models?view=profiles&new=true')}
					>
						<Plus class="size-4" />
						{m.models_profiles_add()}
					</button>
				{/if}
			</div>
		{:else}
			<div class="flex flex-col gap-2">
				<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
					<Search
						value={query}
						class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
						onChange={updateQuery}
						placeholder={m.models_profiles_search_placeholder()}
					/>
				</div>

				<Table
					data={filteredRows}
					fields={['name', 'subjectsLabel', 'modelCount', 'budgetCount', 'guardrailCount']}
					headers={[
						{ property: 'name', title: m.core_name() },
						{ property: 'subjectsLabel', title: m.models_profiles_col_users_groups() },
						{ property: 'modelCount', title: m.models_profiles_col_models() },
						{ property: 'budgetCount', title: m.models_profiles_col_budget() },
						{ property: 'guardrailCount', title: m.models_profiles_applied_guardrails() }
					]}
					sortable={['name', 'subjectsLabel', 'modelCount', 'budgetCount', 'guardrailCount']}
					sortValue={(property, row) =>
						property === 'modelCount' && row.allModels ? Number.POSITIVE_INFINITY : undefined}
					onClickRow={(row, isCtrlClick) =>
						openUrl(resolve('/models/profiles/[id]', { id: row.id }), isCtrlClick)}
					onSort={setSortUrlParams}
					{initSort}
				>
					{#snippet onRenderColumn(property, row)}
						{#if property === 'name'}
							<a
								href={resolve('/models/profiles/[id]', { id: row.id })}
								class="truncate text-base-content"
								aria-label={m.models_profiles_edit_item({ name: row.name })}
								onclick={(e) => e.stopPropagation()}
							>
								{row.name}
							</a>
						{:else if property === 'subjectsLabel'}
							<span class="truncate" title={row.subjectsTitle || undefined}
								>{row.subjectsLabel}</span
							>
						{:else if property === 'modelCount'}
							{#if row.allModels}
								{m.models_budgets_all_models()}
							{:else}
								<span class="tabular-nums">{row.modelCount}</span>
							{/if}
						{:else if property === 'budgetCount'}
							<span class="tabular-nums">{row.budgetCount}</span>
						{:else if property === 'guardrailCount'}
							<span class="tabular-nums">{row.guardrailCount}</span>
						{/if}
					{/snippet}
					{#snippet actions(row)}
						{#if !readonly}
							<IconButton
								variant="danger"
								tooltip={{ text: m.models_delete_profile(), placement: 'bottom' }}
								onclick={() => (toDelete = row)}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					{/snippet}
				</Table>

				<p class="text-muted-content flex items-start gap-2 text-xs">
					<Info class="mt-0.5 size-3 shrink-0" />
					<span>{m.models_profiles_intro()}</span>
				</p>
			</div>
		{/if}
	</div>
{/if}

<Confirm
	show={!!toDelete}
	msg={toDelete?.displayName
		? m.models_profiles_delete_confirm({ name: toDelete.displayName })
		: m.models_profiles_delete_confirm_unnamed()}
	onsuccess={async () => {
		if (toDelete) await AdminService.deleteModelAccessPolicy(toDelete.id);
		toDelete = undefined;
		await accessibleModels.refresh();
		await load();
	}}
	oncancel={() => (toDelete = undefined)}
/>
